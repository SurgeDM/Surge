//go:build !darwin && !linux && !windows && !android

package userservice

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedManager(t *testing.T) {
	mgr, err := New("dummy")
	if err != nil {
		t.Fatalf("unexpected error from New: %v", err)
	}

	ctx := context.Background()
	if err := mgr.Install(ctx); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Install error = %v, want %v", err, ErrUnsupportedPlatform)
	}
	if err := mgr.Uninstall(ctx); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Uninstall error = %v, want %v", err, ErrUnsupportedPlatform)
	}
	if err := mgr.Start(ctx); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Start error = %v, want %v", err, ErrUnsupportedPlatform)
	}
	if err := mgr.Stop(ctx); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Stop error = %v, want %v", err, ErrUnsupportedPlatform)
	}
	if err := mgr.Restart(ctx); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Restart error = %v, want %v", err, ErrUnsupportedPlatform)
	}
	st, err := mgr.Status(ctx)
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("Status error = %v, want %v", err, ErrUnsupportedPlatform)
	}
	if st != NotInstalled {
		t.Errorf("Status = %v, want %v", st, NotInstalled)
	}
}
