//go:build !windows

package state

import "os"

// retryRemove is a no-op wrapper on non-Windows platforms.
func retryRemove(path string) error {
	return os.Remove(path)
}
