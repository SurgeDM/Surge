package idempotency

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestConcurrentReplayAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	ledger := New(path)
	first, owned, err := ledger.Reserve(context.Background(), "scope:key", "request")
	if err != nil || !owned {
		t.Fatalf("reserve: %v %v", owned, err)
	}
	results := make(chan Record, 8)
	for range 8 {
		go func() {
			r, owner, e := ledger.Reserve(context.Background(), "scope:key", "request")
			if e != nil || owner {
				results <- Record{}
				return
			}
			results <- r
		}()
	}
	if err := ledger.Finish("scope:key", true); err != nil {
		t.Fatal(err)
	}
	for range 8 {
		if r := <-results; r.ID != first.ID {
			t.Fatalf("duplicate identity: %+v", r)
		}
	}
	r, owned, err := New(path).Reserve(context.Background(), "scope:key", "request")
	if err != nil || owned || r.ID != first.ID {
		t.Fatalf("restart lost receipt: %+v %v", r, err)
	}
	if _, _, err := ledger.Reserve(context.Background(), "scope:key", "different"); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict: %v", err)
	}
	scoped, owned, err := ledger.Reserve(context.Background(), "other:key", "request")
	if err != nil || !owned || scoped.ID == first.ID {
		t.Fatal("authentication scopes merged")
	}
	unknown, owned, err := New(path).Reserve(context.Background(), "other:key", "request")
	if !errors.Is(err, ErrUnknown) || owned || unknown.ID != scoped.ID {
		t.Fatalf("ambiguous reservation repeated: %+v %v", unknown, err)
	}
}
func TestCanceledDuplicateDoesNotCancelOwner(t *testing.T) {
	ledger := New(filepath.Join(t.TempDir(), "ledger.json"))
	first, _, err := ledger.Reserve(context.Background(), "key", "hash")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := ledger.Reserve(ctx, "key", "hash"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := ledger.Finish("key", true); err != nil {
		t.Fatal(err)
	}
	replay, _, err := ledger.Reserve(context.Background(), "key", "hash")
	if err != nil || replay.ID != first.ID {
		t.Fatal("duplicate observer canceled owner")
	}
}
