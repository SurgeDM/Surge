package concurrent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNetworkWorkerPool_ReusesWorkersAcrossRuns(t *testing.T) {
	pool := NewNetworkWorkerPool(1)
	defer pool.Shutdown()

	var runs atomic.Int32

	for i := 0; i < 2; i++ {
		results := pool.Run(context.Background(), 1, func(workerID int) error {
			if workerID != 0 {
				t.Fatalf("workerID = %d, want 0", workerID)
			}
			runs.Add(1)
			time.Sleep(10 * time.Millisecond)
			return nil
		})

		for err := range results {
			if err != nil {
				t.Fatalf("run %d returned error: %v", i, err)
			}
		}
	}

	if got := runs.Load(); got != 2 {
		t.Fatalf("runs = %d, want 2", got)
	}
}
