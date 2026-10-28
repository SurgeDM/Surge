package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
)

func TestAddWithPreparation_RejectionsHaveNoSideEffects(t *testing.T) {
	for _, state := range []string{"queued", "active", "shutdown"} {
		t.Run(state, func(t *testing.T) {
			cfg := types.DownloadRecord{ID: "same-id"}
			pool := newTestSchedulerWithoutWorkers(nil)
			t.Cleanup(pool.GracefulShutdown)
			switch state {
			case "queued":
				if !pool.Add(cfg) {
					t.Fatal("initial add rejected")
				}
			case "active":
				pool.downloads[cfg.ID] = &activeDownload{config: cfg}
			case "shutdown":
				pool.isShuttingDown = true
			}
			called := false
			accepted, err := pool.AddWithPreparation(cfg, func() error { called = true; return nil })
			if accepted || err != nil || called {
				t.Fatalf("accepted=%v err=%v preparation called=%v", accepted, err, called)
			}
		})
	}
}

func TestAddWithPreparation_FailureDoesNotQueue(t *testing.T) {
	pool := newTestSchedulerWithoutWorkers(nil)
	t.Cleanup(pool.GracefulShutdown)
	want := errors.New("persistence failed")
	cfg := types.DownloadRecord{ID: "new-id"}
	accepted, err := pool.AddWithPreparation(cfg, func() error { return want })
	if accepted || !errors.Is(err, want) {
		t.Fatalf("accepted=%v err=%v", accepted, err)
	}
	if len(pool.queued) != 0 || len(pool.queueOrder) != 0 {
		t.Fatal("failed preparation left queued task")
	}
	if !pool.Add(cfg) {
		t.Fatal("failed preparation retained ownership")
	}
}

func TestAddWithPreparation_PrecedesRunnableTask(t *testing.T) {
	pool := newTestSchedulerWithoutWorkers(nil)
	t.Cleanup(pool.GracefulShutdown)
	cfg := types.DownloadRecord{ID: "new-id"}
	called := false
	accepted, err := pool.AddWithPreparation(cfg, func() error {
		called = true
		if len(pool.queued) != 0 || len(pool.queueOrder) != 0 {
			t.Errorf("task runnable before preparation")
			return errors.New("task runnable before preparation")
		}
		return nil
	})
	if !accepted || err != nil || !called || pool.queued[cfg.ID] == nil {
		t.Fatalf("accepted=%v err=%v preparation called=%v", accepted, err, called)
	}
}

func TestBeforeStartReleasesLockAndReservesAdmission(t *testing.T) {
	pool := newTestSchedulerWithoutWorkers(nil)
	t.Cleanup(pool.GracefulShutdown)
	cfg := types.DownloadRecord{ID: "resuming"}
	accepted, err := pool.AddWithBeforeStart(cfg, func() error {
		if !pool.mu.TryLock() {
			return errors.New("scheduler lock held during event publication")
		}
		pool.mu.Unlock()
		if status := pool.GetStatus(cfg.ID); status == nil || status.Status != "queued" {
			return errors.New("reservation not visible")
		}
		duplicateCalled := false
		if ok, _ := pool.AddWithBeforeStart(cfg, func() error { duplicateCalled = true; return nil }); ok || duplicateCalled {
			return errors.New("duplicate admission was accepted")
		}
		if !pool.Add(types.DownloadRecord{ID: "other"}) {
			return errors.New("unrelated download blocked")
		}
		// A worker must skip the reserved download even though it is first in
		// queue order, and remain free to claim another download.
		task := pool.waitForTask()
		if task.cfg.ID != "other" {
			return errors.New("resumed download runnable before publication")
		}
		pool.mu.Lock()
		task.inFlight = false // No worker was launched; return this test claim.
		pool.mu.Unlock()
		return nil
	})
	if !accepted || err != nil {
		t.Fatalf("accepted=%v error=%v", accepted, err)
	}
	if pool.queued[cfg.ID].inFlight {
		t.Fatal("reservation did not become runnable")
	}
}

func TestBeforeStartFailureReleasesReservation(t *testing.T) {
	pool := newTestSchedulerWithoutWorkers(nil)
	t.Cleanup(pool.GracefulShutdown)
	want := errors.New("event publication failed")
	cfg := types.DownloadRecord{ID: "resuming"}
	accepted, err := pool.AddWithBeforeStart(cfg, func() error { return want })
	if accepted || !errors.Is(err, want) {
		t.Fatalf("accepted=%v error=%v", accepted, err)
	}
	if !pool.Add(cfg) {
		t.Fatal("failed publication retained reservation")
	}
}

func TestBeforeStartCancellationDoesNotReviveDownload(t *testing.T) {
	for _, action := range []string{"pause", "cancel", "replace"} {
		t.Run(action, func(t *testing.T) {
			pool := newTestSchedulerWithoutWorkers(nil)
			cfg := types.DownloadRecord{ID: "resuming"}
			accepted, err := pool.AddWithBeforeStart(cfg, func() error {
				if action == "pause" {
					pool.Pause(cfg.ID)
				} else {
					pool.Cancel(cfg.ID)
				}
				if action == "replace" && !pool.Add(cfg) {
					return errors.New("replacement rejected")
				}
				return nil
			})
			if accepted || err != nil {
				t.Fatalf("cancelled reservation: accepted=%v error=%v", accepted, err)
			}
			if action != "replace" && pool.GetStatus(cfg.ID) != nil {
				t.Fatal("cancelled task revived")
			}
			if action == "replace" && pool.GetStatus(cfg.ID) == nil {
				t.Fatal("replacement removed")
			}
			awaitShutdown(t, pool) // Also verifies callback and cancellation balance wg.
		})
	}
}

func TestBeforeStartShutdownDoesNotStartOrLeakReservation(t *testing.T) {
	pool := newTestSchedulerWithoutWorkers(nil)
	stopped := make(chan struct{})
	accepted, err := pool.AddWithBeforeStart(types.DownloadRecord{ID: "resuming"}, func() error {
		go func() { pool.GracefulShutdown(); close(stopped) }()
		pool.mu.Lock()
		for !pool.isShuttingDown {
			pool.taskCond.Wait()
		}
		pool.mu.Unlock()
		return nil
	})
	if accepted || err != nil {
		t.Fatalf("shutdown reservation: accepted=%v error=%v", accepted, err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("shutdown leaked reservation wait-group count")
	}
	if pool.GetStatus("resuming") != nil {
		t.Fatal("shutdown task revived")
	}
}
