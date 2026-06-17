package state

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	baseDir    string
	configured bool
	masterMu   sync.RWMutex
)

// Configure sets the base directory for the custom state backend.
// The path parameter was previously the path to the sqlite file (e.g. downloads.db).
// We use filepath.Dir(path) to get the state directory.
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
	return nil
}

func initDB() error {
	masterMu.Lock()
	defer masterMu.Unlock()
	if err := ensureDirs(); err != nil {
		return err
	}

	// Deferred cleanup of orphans
	cleanupOrphans(baseDir)
	cleanupOrphans(filepath.Join(baseDir, "details"))

	return nil
}

func cleanupOrphans(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) >= 4 && entry.Name()[:4] == ".tmp" {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

func CloseDB() {
	masterMu.Lock()
	defer masterMu.Unlock()
	baseDir = ""
	configured = false
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
