//go:build !windows

package utils

import "os"

// RemoveFile removes a file from disk. On non-Windows platforms this is a
// direct call to os.Remove; no retry is needed because POSIX unlink semantics
// allow removing an open file (the directory entry is removed immediately and
// the data persists until the last file descriptor is closed).
func RemoveFile(path string) error {
	return os.Remove(path)
}
