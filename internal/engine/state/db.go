package state

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	baseDir     string
	configured  bool
	masterMu    sync.RWMutex
	cleanupMu   sync.Mutex
	cleanupOnce sync.Once
)

// Configure sets the base directory for the custom state backend.
// It accepts a file path (e.g., legacy downloads.db) for backward compatibility,
// avoiding a massive refactor of all startup paths that pass a database file path.
// It extracts and uses the parent directory for Gob state.
func Configure(path string) {
	masterMu.Lock()
	defer masterMu.Unlock()
	baseDir = filepath.Dir(path)
	configured = true
}

func ensureDirs() error {
	if !configured || baseDir == "" {
		return fmt.Errorf("state backend not configured")
	}
	detailsDir := filepath.Join(baseDir, "details")
	if err := os.MkdirAll(detailsDir, 0o755); err != nil {
		return err
	}
	cleanupMu.Lock()
	cleanupOnce.Do(func() {
		cleanupOrphans(baseDir)
		cleanupOrphans(detailsDir)
	})
	cleanupMu.Unlock()
	return nil
}

func cleanupOrphans(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".tmp-") {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func CloseDB() {
	masterMu.Lock()
	defer masterMu.Unlock()
	baseDir = ""
	configured = false
	// Reset cleanupOnce under its own dedicated mutex so it is safe to call
	// concurrently with ensureDirs, which also holds cleanupMu when it reads it.
	cleanupMu.Lock()
	cleanupOnce = sync.Once{}
	cleanupMu.Unlock()
}

func atomicWrite(targetPath string, data interface{}) error {
	dir := filepath.Dir(targetPath)
	f, err := os.CreateTemp(dir, ".tmp-*.gob")
	if err != nil {
		return err
	}
	tmpName := f.Name()
	defer func() { _ = os.Remove(tmpName) }() // cleans up if we fail before rename

	if err := gob.NewEncoder(f).Encode(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, targetPath)
}

func loadGob(path string, v interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return gob.NewDecoder(f).Decode(v)
}
