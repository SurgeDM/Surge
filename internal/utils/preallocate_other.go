//go:build !linux && !windows

package utils

import "os"

func preallocateFalloc(file *os.File, size int64) error {
	return file.Truncate(size)
}

func setFallocateForTest(func(fd int, size int64) error) func() {
	return func() {}
}
