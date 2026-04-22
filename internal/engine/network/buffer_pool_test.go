package network

import "testing"

func TestBufferPoolManager_IsolatesBySize(t *testing.T) {
	manager := NewBufferPoolManager()

	small := manager.Get(1024)
	sameSmall := manager.Get(1024)
	large := manager.Get(2048)

	if small != sameSmall {
		t.Fatal("expected same-size requests to reuse the same pool")
	}
	if small == large {
		t.Fatal("expected different-size requests to use different pools")
	}

	buf := small.Get().(*[]byte)
	if got := len(*buf); got != 1024 {
		t.Fatalf("buffer len = %d, want 1024", got)
	}
	small.Put(buf)

	buf = large.Get().(*[]byte)
	if got := len(*buf); got != 2048 {
		t.Fatalf("buffer len = %d, want 2048", got)
	}
	large.Put(buf)
}
