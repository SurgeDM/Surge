package concurrent

import (
	"context"
	"sync"
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

func TestNetworkWorkerPool_ContiguousWorkerIDsOnCancel(t *testing.T) {
	pool := NewNetworkWorkerPool(1)
	defer pool.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Take the only worker slot so the next Run blocks
	started := make(chan struct{})
	go func() {
		_ = pool.Run(ctx, 1, func(workerID int) error {
			started <- struct{}{}
			<-ctx.Done()
			return nil
		})
	}()
	<-started

	// 2. Start a second Run that will block on submission for i=0
	var ids []int
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		results := pool.Run(ctx, 10, func(id int) error {
			mu.Lock()
			ids = append(ids, id)
			mu.Unlock()
			return nil
		})
		for range results {
		}
		close(done)
	}()

	// 3. Cancel the second run while it's blocked on i=0
	time.Sleep(50 * time.Millisecond) // Ensure it's blocked
	cancel()
	<-done

	// Verify contiguous IDs (should be empty as i=0 was blocked)
	mu.Lock()
	defer mu.Unlock()
	for i, id := range ids {
		if id != i {
			t.Fatalf("Non-contiguous workerID at index %d: got %d, want %d", i, id, i)
		}
	}
}
