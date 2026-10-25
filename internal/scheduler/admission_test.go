package scheduler

import (
	"errors"
	"testing"

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
