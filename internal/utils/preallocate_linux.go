//go:build linux

package utils

import (
	"os"
	"syscall"
)

// fallocateFn allocates the byte range [0, size). Tests replace it to force the truncate fallback.
var fallocateFn = func(fd int, size int64) error {
	return syscall.Fallocate(fd, 0, 0, size)
}

// setFallocateForTest replaces the Linux fallocate syscall until restore is called.
func setFallocateForTest(fn func(fd int, size int64) error) (restore func()) {
	prev := fallocateFn
	fallocateFn = fn
	return func() { fallocateFn = prev }
}

// preallocateFalloc attempts physical allocation first and falls back to logical truncation.
func preallocateFalloc(file *os.File, size int64) error {
	if err := fallocateFn(int(file.Fd()), size); err != nil {
		return file.Truncate(size)
	}
	return nil
}
