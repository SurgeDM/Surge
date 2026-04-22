package concurrent

import (
	"context"
	"sync"
)

type networkWorkerJob struct {
	ctx      context.Context
	workerID int
	fn       func(workerID int) error
	results  chan error
	done     *sync.WaitGroup
}

// NetworkWorkerPool reuses a fixed set of goroutines for chunk execution across downloads.
type NetworkWorkerPool struct {
	size     int
	jobs     chan networkWorkerJob
	stopOnce sync.Once
}

func NewNetworkWorkerPool(size int) *NetworkWorkerPool {
	if size < 1 {
		size = 1
	}

	pool := &NetworkWorkerPool{
		size: size,
		jobs: make(chan networkWorkerJob),
	}

	for i := 0; i < size; i++ {
		go pool.worker()
	}

	return pool
}

func (p *NetworkWorkerPool) Run(ctx context.Context, workerCount int, fn func(workerID int) error) <-chan error {
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > p.size {
		workerCount = p.size
	}

	results := make(chan error, workerCount)
	var done sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		done.Add(1)
		job := networkWorkerJob{
			ctx:      ctx,
			workerID: i,
			fn:       fn,
			results:  results,
			done:     &done,
		}

		select {
		case p.jobs <- job:
		case <-ctx.Done():
			done.Done()
		}
	}

	go func() {
		done.Wait()
		close(results)
	}()

	return results
}

func (p *NetworkWorkerPool) Size() int {
	return p.size
}

func (p *NetworkWorkerPool) Shutdown() {
	p.stopOnce.Do(func() {
		close(p.jobs)
	})
}

func (p *NetworkWorkerPool) worker() {
	for job := range p.jobs {
		err := job.fn(job.workerID)
		if err != nil && err != context.Canceled {
			job.results <- err
		}
		job.done.Done()
	}
}
