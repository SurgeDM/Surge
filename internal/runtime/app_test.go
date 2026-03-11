package runtime

import (
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/processing"
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

func TestLifecycleForService_ReturnsNilForStaleService(t *testing.T) {
	currentService := core.NewLocalDownloadServiceWithInput(nil, nil)
	staleService := core.NewLocalDownloadServiceWithInput(nil, nil)
	t.Cleanup(func() {
		_ = currentService.Shutdown()
		_ = staleService.Shutdown()
	})

	app := NewEmpty()
	app.ApplyComponents(Components{
		Service:   currentService,
		Lifecycle: processing.NewLifecycleManager(nil, nil),
	})

	lifecycle, err := app.LifecycleForService(staleService)
	if err != nil {
		t.Fatalf("LifecycleForService() error = %v", err)
	}
	if lifecycle != nil {
		t.Fatal("expected stale service lookup to fall back with nil lifecycle")
	}
}

func TestEnsureLocalServiceAndLifecycle_DoesNotRewireExistingHooks(t *testing.T) {
	settings := config.DefaultSettings()
	app := NewLocal(settings)
	t.Cleanup(func() {
		_ = app.Shutdown()
	})

	if err := app.EnsureLocalServiceAndLifecycle(); err != nil {
		t.Fatalf("EnsureLocalServiceAndLifecycle() error = %v", err)
	}

	localService, ok := app.Service().(*core.LocalDownloadService)
	if !ok {
		t.Fatal("expected local download service")
	}

	pauseSentinel := func(string) error { return errors.New("pause sentinel") }
	resumeSentinel := func(string) error { return errors.New("resume sentinel") }
	resumeBatchSentinel := func([]string) []error { return []error{errors.New("resume batch sentinel")} }

	localService.PauseFunc = pauseSentinel
	localService.ResumeFunc = resumeSentinel
	localService.ResumeBatchFunc = resumeBatchSentinel

	if err := app.EnsureLocalServiceAndLifecycle(); err != nil {
		t.Fatalf("EnsureLocalServiceAndLifecycle() second call error = %v", err)
	}

	if got := reflect.ValueOf(localService.PauseFunc).Pointer(); got != reflect.ValueOf(pauseSentinel).Pointer() {
		t.Fatal("expected PauseFunc wiring to remain unchanged")
	}
	if got := reflect.ValueOf(localService.ResumeFunc).Pointer(); got != reflect.ValueOf(resumeSentinel).Pointer() {
		t.Fatal("expected ResumeFunc wiring to remain unchanged")
	}
	if got := reflect.ValueOf(localService.ResumeBatchFunc).Pointer(); got != reflect.ValueOf(resumeBatchSentinel).Pointer() {
		t.Fatal("expected ResumeBatchFunc wiring to remain unchanged")
	}
}
