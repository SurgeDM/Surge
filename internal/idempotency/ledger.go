// Package idempotency reserves stable download identities before submission.
// It stores hashes and receipts, never source URLs or authentication material.
package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrConflict = errors.New("idempotency key conflicts with an earlier request")
var ErrUnknown = errors.New("earlier submission outcome is unknown; inspect its download ID")
var ErrFull = errors.New("idempotency ledger is full")

const MaxEntries = 10000
const Retention = 30 * 24 * time.Hour

type Record struct {
	Hash     string    `json:"request_hash"`
	ID       string    `json:"id"`
	Accepted bool      `json:"accepted"`
	Created  time.Time `json:"created"`
}
type diskState struct {
	Version int               `json:"version"`
	Records map[string]Record `json:"records"`
}
type Ledger struct {
	mu      sync.Mutex
	path    string
	records map[string]Record
	running map[string]chan struct{}
}

func New(path string) *Ledger { return &Ledger{path: path, running: make(map[string]chan struct{})} }

// Reserve serializes identical submissions without holding a lock during probes.
// Pending records surviving a restart fail closed instead of repeating a mutation.
func (l *Ledger) Reserve(ctx context.Context, key, hash string) (Record, bool, error) {
	for {
		l.mu.Lock()
		if err := l.load(); err != nil {
			l.mu.Unlock()
			return Record{}, false, err
		}
		if r, exists := l.records[key]; exists {
			if r.Hash != hash {
				l.mu.Unlock()
				return r, false, ErrConflict
			}
			if done, active := l.running[key]; active {
				l.mu.Unlock()
				select {
				case <-done:
					continue
				case <-ctx.Done():
					return r, false, ctx.Err()
				}
			}
			l.mu.Unlock()
			if !r.Accepted {
				return r, false, ErrUnknown
			}
			return r, false, nil
		}
		// Expire accepted receipts only. Ambiguous reservations are never evicted.
		for k, r := range l.records {
			if r.Accepted && time.Since(r.Created) > Retention {
				delete(l.records, k)
			}
		}
		if len(l.records) >= MaxEntries {
			l.mu.Unlock()
			return Record{}, false, ErrFull
		}
		r := Record{Hash: hash, ID: uuid.NewString(), Created: time.Now().UTC()}
		l.records[key] = r
		if err := l.save(); err != nil {
			delete(l.records, key)
			l.mu.Unlock()
			return Record{}, false, err
		}
		l.running[key] = make(chan struct{})
		l.mu.Unlock()
		return r, true, nil
	}
}

// Finish keeps failures ambiguous: a probe may have persisted work before failing.
func (l *Ledger) Finish(key string, accepted bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r := l.records[key]
	r.Accepted = accepted
	l.records[key] = r
	err := l.save()
	if done, ok := l.running[key]; ok {
		close(done)
		delete(l.running, key)
	}
	return err
}
func (l *Ledger) load() error {
	if l.records != nil {
		return nil
	}
	f, err := os.Open(l.path)
	if errors.Is(err, os.ErrNotExist) {
		l.records = make(map[string]Record)
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var disk diskState
	decoder := json.NewDecoder(io.LimitReader(f, 8<<20))
	if err := decoder.Decode(&disk); err != nil {
		return err
	}
	if disk.Version != 1 || disk.Records == nil || len(disk.Records) > MaxEntries {
		return errors.New("invalid idempotency ledger")
	}
	l.records = disk.Records
	return nil
}
func (l *Ledger) save() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(l.path), ".idempotency-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if err = json.NewEncoder(f).Encode(diskState{Version: 1, Records: l.records}); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, l.path)
}
