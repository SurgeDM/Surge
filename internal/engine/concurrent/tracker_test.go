package concurrent

import (
	"sync/atomic"
	"testing"

	"github.com/surge-downloader/surge/internal/engine/types"
)

func TestRetryTracker_AbortsAfterRepeatedNoProgress(t *testing.T) {
	tracker := NewRetryTracker(3)
	qTask := queuedTask{
		task:      types.Task{Offset: 0, Length: 1024},
		lineageID: 1,
	}

	for i := 0; i < 3; i++ {
		if got := tracker.Evaluate(qTask); got != DispositionRequeue {
			t.Fatalf("Evaluate() on failure %d = %v, want requeue", i+1, got)
		}
	}

	if got := tracker.Evaluate(qTask); got != DispositionAbort {
		t.Fatalf("Evaluate() on 4th no-progress failure = %v, want abort", got)
	}
}

func TestRetryTracker_ProgressResetsStallBudget(t *testing.T) {
	tracker := NewRetryTracker(3)
	qTask := queuedTask{
		task:      types.Task{Offset: 0, Length: 1024},
		lineageID: 7,
	}

	if got := tracker.Evaluate(qTask); got != DispositionRequeue {
		t.Fatalf("initial Evaluate() = %v, want requeue", got)
	}
	if got := tracker.Evaluate(qTask); got != DispositionRequeue {
		t.Fatalf("second Evaluate() = %v, want requeue", got)
	}

	advanced := qTask.withTask(types.Task{Offset: 256, Length: 768})
	if got := tracker.Evaluate(advanced); got != DispositionRequeue {
		t.Fatalf("Evaluate() after progress = %v, want requeue", got)
	}

	if got := tracker.Evaluate(advanced); got != DispositionRequeue {
		t.Fatalf("Evaluate() after progress reset = %v, want requeue", got)
	}
}

func TestPendingResultCounter_BalancesAbortPath(t *testing.T) {
	var pending pendingResultCounter

	pending.Begin()
	if got := pending.Load(); got != 1 {
		t.Fatalf("Load() after Begin = %d, want 1", got)
	}

	pending.Complete()
	if got := pending.Load(); got != 0 {
		t.Fatalf("Load() after Complete = %d, want 0", got)
	}
}

func TestLoadVerifiedBytes_UsesFallbackWhenStateNil(t *testing.T) {
	var fallback atomic.Int64
	fallback.Store(123)

	if got := loadVerifiedBytes(nil, &fallback); got != 123 {
		t.Fatalf("loadVerifiedBytes(nil, fallback) = %d, want 123", got)
	}

	state := types.NewProgressState("verified", 456)
	state.VerifiedProgress.Store(456)
	fallback.Store(999)

	if got := loadVerifiedBytes(state, &fallback); got != 456 {
		t.Fatalf("loadVerifiedBytes(state, fallback) = %d, want 456", got)
	}
}
