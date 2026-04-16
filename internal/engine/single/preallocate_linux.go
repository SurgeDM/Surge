//go:build linux

package single

import (
	"os"
	"syscall"
)

// preallocateFile attempts physical allocation first and falls back to logical truncation.
func preallocateFile(file *os.File, size int64) error {
	if size <= 0 {
		return nil
	}

	if err := syscall.Fallocate(int(file.Fd()), 0, 0, size); err == nil { //nolint:gosec // int(file.Fd()) is safe on 64-bit linux
		return nil
	}

	return file.Truncate(size)
}
