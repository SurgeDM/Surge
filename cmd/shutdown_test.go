package cmd

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/surge-downloader/surge/internal/engine/types"
)

func TestExecuteGlobalShutdown_Once(t *testing.T) {
	var calls int32
	resetGlobalShutdownCoordinatorForTest(func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	t.Cleanup(func() { resetGlobalShutdownCoordinatorForTest(nil) })

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := executeGlobalShutdown("test"); err != nil {
				t.Errorf("executeGlobalShutdown returned error: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("shutdown function calls = %d, want 1", got)
	}
}

type fakeShutdownService struct {
	fakeRemoteDownloadService
	onShutdown func()
}

func (f *fakeShutdownService) StreamEvents(context.Context) (<-chan interface{}, func(), error) {
	ch := make(chan interface{})
	return ch, func() { close(ch) }, nil
}

func (f *fakeShutdownService) GetStatus(string) (*types.DownloadStatus, error) {
	return nil, nil
}

func (f *fakeShutdownService) Shutdown() error {
	if f.onShutdown != nil {
		f.onShutdown()
	}
	return nil
}

func TestDefaultGlobalShutdown_CleanupBeforeService(t *testing.T) {
	var order []string
	GlobalService = &fakeShutdownService{
		onShutdown: func() {
			order = append(order, "shutdown")
		},
	}
	GlobalLifecycleCleanup = func() {
		order = append(order, "cleanup")
	}
	t.Cleanup(func() {
		GlobalService = nil
		GlobalLifecycle = nil
		_ = takeLifecycleCleanup()
		resetGlobalShutdownCoordinatorForTest(nil)
	})

	if err := defaultGlobalShutdown(); err != nil {
		t.Fatalf("defaultGlobalShutdown failed: %v", err)
	}

	if len(order) != 2 || order[0] != "cleanup" || order[1] != "shutdown" {
		t.Fatalf("shutdown order = %v, want [cleanup shutdown]", order)
	}
}

func TestDefaultGlobalShutdown_CancelsEnqueueContext(t *testing.T) {
	resetGlobalEnqueueContext()
	ctx := currentEnqueueContext()

	if err := defaultGlobalShutdown(); err != nil {
		t.Fatalf("defaultGlobalShutdown failed: %v", err)
	}

	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected shutdown to cancel the shared enqueue context")
	}
}
