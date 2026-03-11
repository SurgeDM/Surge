package runtime

import (
	"sync"
	"testing"

	"github.com/surge-downloader/surge/internal/config"
)

func TestAppResetEnqueueContextReplacesCanceledContext(t *testing.T) {
	app := NewEmpty()

	ctx1 := app.EnqueueContext()
	cancel1 := app.EnqueueCancel()
	cancel1()

	app.ResetEnqueueContext()
	ctx2 := app.EnqueueContext()

	if ctx1 == ctx2 {
		t.Fatal("expected reset to create a new enqueue context")
	}
	if err := ctx1.Err(); err == nil {
		t.Fatal("expected previous context to be canceled")
	}
	if err := ctx2.Err(); err != nil {
		t.Fatalf("expected new context to be active, got %v", err)
	}
}

func TestNewLocalInitializesPoolAndProgressChannel(t *testing.T) {
	settings := config.DefaultSettings()
	settings.Network.MaxConcurrentDownloads = 2

	app := NewLocal(settings)
	t.Cleanup(func() {
		_ = app.Shutdown()
	})

	if app.Pool() == nil {
		t.Fatal("expected local app to initialize worker pool")
	}
	if app.ProgressCh() == nil {
		t.Fatal("expected local app to initialize progress channel")
	}
}

func TestEnsureLocalServiceAndLifecycle_ConcurrentInitializationUsesOneService(t *testing.T) {
	settings := config.DefaultSettings()
	app := NewLocal(settings)
	t.Cleanup(func() {
		_ = app.Shutdown()
	})

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := app.EnsureLocalServiceAndLifecycle(); err != nil {
				t.Errorf("EnsureLocalServiceAndLifecycle failed: %v", err)
			}
		}()
	}
	wg.Wait()

	if app.Service() == nil {
		t.Fatal("expected service to be initialized")
	}
	if app.CurrentLifecycle() == nil {
		t.Fatal("expected lifecycle manager to be initialized")
	}
}
