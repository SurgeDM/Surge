package probe

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLockProbeHostReleasesUnusedLock(t *testing.T) {
	release, err := lockProbeHost(context.Background(), "https://example.com/file")
	if err != nil {
		t.Fatal(err)
	}
	release()

	probeHostLocks.Lock()
	_, retained := probeHostLocks.hosts["example.com"]
	probeHostLocks.Unlock()
	if retained {
		t.Fatal("released host lock was retained")
	}
}

func TestProbeWaitForSameHostHonorsContext(t *testing.T) {
	const rawURL = "https://example.com/held"
	release, err := lockProbeHost(context.Background(), rawURL)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := ProbeServer(ctx, "https://example.com/other", "", nil)
		result <- err
	}()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline while host was locked, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("probe waited past its deadline for the host lock")
	}

	probeHostLocks.Lock()
	refs := probeHostLocks.hosts["example.com"].refs
	probeHostLocks.Unlock()
	if refs != 1 {
		t.Fatalf("expected only the original holder to remain, got %d references", refs)
	}
}
