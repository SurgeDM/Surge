package cmd

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/surge-downloader/surge/internal/processing"
	runtimeapp "github.com/surge-downloader/surge/internal/runtime"
)

func TestCurrentApp_ReplacesOutOfSyncApp_ShutsDownPreviousApp(t *testing.T) {
	var shutdownCalls int32
	var cleanupCalls int32

	previousService := &fakeShutdownService{
		onShutdown: func() {
			atomic.AddInt32(&shutdownCalls, 1)
		},
	}

	previousApp := runtimeapp.NewEmpty()
	previousApp.ApplyComponents(runtimeapp.Components{
		Service:          previousService,
		Lifecycle:        processing.NewLifecycleManager(nil, nil),
		LifecycleCleanup: func() { atomic.AddInt32(&cleanupCalls, 1) },
	})

	globalApp = previousApp
	GlobalService = &fakeShutdownService{}
	GlobalLifecycle = nil
	GlobalLifecycleCleanup = nil
	GlobalPool = nil
	GlobalProgressCh = nil

	t.Cleanup(func() {
		globalApp = nil
		GlobalService = nil
		GlobalLifecycle = nil
		GlobalLifecycleCleanup = nil
		GlobalPool = nil
		GlobalProgressCh = nil
	})

	app := currentApp()
	if app == previousApp {
		t.Fatal("expected currentApp to replace the stale runtime app")
	}
	if got := atomic.LoadInt32(&shutdownCalls); got != 1 {
		t.Fatalf("shutdown calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&cleanupCalls); got != 1 {
		t.Fatalf("cleanup calls = %d, want 1", got)
	}
}

func TestCurrentApp_ConcurrentCallersShareOneReplacementApp(t *testing.T) {
	var shutdownCalls int32

	shutdownStarted := make(chan struct{})
	releaseShutdown := make(chan struct{})
	previousService := &fakeShutdownService{
		onShutdown: func() {
			atomic.AddInt32(&shutdownCalls, 1)
			close(shutdownStarted)
			<-releaseShutdown
		},
	}

	previousApp := runtimeapp.NewEmpty()
	previousApp.ApplyComponents(runtimeapp.Components{
		Service: previousService,
	})

	globalApp = previousApp
	GlobalService = &fakeShutdownService{}
	GlobalLifecycle = nil
	GlobalLifecycleCleanup = nil
	GlobalPool = nil
	GlobalProgressCh = nil

	t.Cleanup(func() {
		globalApp = nil
		GlobalService = nil
		GlobalLifecycle = nil
		GlobalLifecycleCleanup = nil
		GlobalPool = nil
		GlobalProgressCh = nil
	})

	const callers = 16
	start := make(chan struct{})
	apps := make(chan *runtimeapp.App, callers)

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			apps <- currentApp()
		}()
	}

	close(start)
	<-shutdownStarted
	close(releaseShutdown)
	wg.Wait()
	close(apps)

	var replacement *runtimeapp.App
	for app := range apps {
		if app == previousApp {
			t.Fatal("expected currentApp to replace the stale runtime app")
		}
		if replacement == nil {
			replacement = app
			continue
		}
		if app != replacement {
			t.Fatal("expected concurrent callers to receive the same replacement runtime app")
		}
	}

	if got := atomic.LoadInt32(&shutdownCalls); got != 1 {
		t.Fatalf("shutdown calls = %d, want 1", got)
	}
}

func TestCurrentApp_DoesNotBlockReplacementWhilePreviousAppShutsDown(t *testing.T) {
	var shutdownCalls int32

	shutdownStarted := make(chan struct{})
	releaseShutdown := make(chan struct{})
	previousService := &fakeShutdownService{
		onShutdown: func() {
			atomic.AddInt32(&shutdownCalls, 1)
			close(shutdownStarted)
			<-releaseShutdown
		},
	}

	previousApp := runtimeapp.NewEmpty()
	previousApp.ApplyComponents(runtimeapp.Components{
		Service: previousService,
	})

	globalApp = previousApp
	GlobalService = &fakeShutdownService{}
	GlobalLifecycle = nil
	GlobalLifecycleCleanup = nil
	GlobalPool = nil
	GlobalProgressCh = nil

	t.Cleanup(func() {
		globalApp = nil
		GlobalService = nil
		GlobalLifecycle = nil
		GlobalLifecycleCleanup = nil
		GlobalPool = nil
		GlobalProgressCh = nil
	})

	firstResult := make(chan *runtimeapp.App, 1)
	go func() {
		firstResult <- currentApp()
	}()

	<-shutdownStarted

	secondResult := make(chan *runtimeapp.App, 1)
	go func() {
		secondResult <- currentApp()
	}()

	var replacement *runtimeapp.App
	select {
	case replacement = <-secondResult:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("expected replacement app to be visible before previous shutdown completes")
	}

	if replacement == previousApp {
		t.Fatal("expected currentApp to return the replacement app")
	}

	close(releaseShutdown)

	if first := <-firstResult; first != replacement {
		t.Fatal("expected both callers to observe the same replacement app")
	}
	if got := atomic.LoadInt32(&shutdownCalls); got != 1 {
		t.Fatalf("shutdown calls = %d, want 1", got)
	}
}
