//go:build !windows

package utils

import (
	"errors"
	"os"
	"runtime"
	"strings"
)

// RemoveFile removes a file from disk. On non-Windows platforms this is a
// direct call to os.Remove; no retry is needed because POSIX unlink semantics
// allow removing an open file (the directory entry is removed immediately and
// the data persists until the last file descriptor is closed).
func RemoveFile(path string) error {
	if strings.HasSuffix(path, ".surge") {
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		Debug("INTERCEPTED DELETION OF %s\nStack:\n%s", path, string(buf[:n]))
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
