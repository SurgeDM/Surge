package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// InstanceLock owns a single acquired instance lock.
type InstanceLock struct {
	flock *flock.Flock
	path  string
}

// TryAcquireInstanceLock tries to acquire the instance lock in runtimeDir.
// It returns acquired=false when another instance already owns the lock.
func TryAcquireInstanceLock(runtimeDir string) (*InstanceLock, bool, error) {
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return nil, false, fmt.Errorf("create lock directory %q: %w", runtimeDir, err)
	}

	lockPath := filepath.Join(runtimeDir, "surge.lock")
	fileLock := flock.New(lockPath)

	locked, err := fileLock.TryLock()
	if err != nil {
		return nil, false, fmt.Errorf("acquire instance lock %q: %w", lockPath, err)
	}

	if locked {
		return &InstanceLock{
			flock: fileLock,
			path:  lockPath,
		}, true, nil
	}

	return nil, false, nil
}

// Release releases the lock owned by this handle.
func (l *InstanceLock) Release() error {
	if l == nil || l.flock == nil {
		return nil
	}
	if err := l.flock.Unlock(); err != nil {
		return fmt.Errorf("release instance lock %q: %w", l.path, err)
	}
	return nil
}
