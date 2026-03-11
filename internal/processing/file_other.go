//go:build !windows

package processing

import "os"

// retryRemove is a no-op wrapper on non-Windows platforms where file locking
// does not prevent deletion of open files.
func retryRemove(path string) error {
	return os.Remove(path)
}

// retryRename is a no-op wrapper on non-Windows platforms.
func retryRename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
