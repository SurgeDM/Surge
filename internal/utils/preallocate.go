package utils

import (
	"os"
	"strings"
)

const (
	// FileAllocationFalloc reserves space without a userspace zero-fill.
	// Linux uses fallocate (truncate if that fails). Windows marks the file
	// sparse and then truncates so NTFS does not zero-fill.
	FileAllocationFalloc = "falloc"
	// FileAllocationPrealloc is truncate-only on every OS.
	FileAllocationPrealloc = "prealloc"
)

// NormalizeFileAllocation returns prealloc only for that mode.
// Empty, unknown, and differently cased values become falloc.
func NormalizeFileAllocation(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), FileAllocationPrealloc) {
		return FileAllocationPrealloc
	}
	return FileAllocationFalloc
}

// PreallocateFile sets file's logical size to size.
// size <= 0 is a no-op and does not grow the file.
func PreallocateFile(file *os.File, size int64, mode string) error {
	if size <= 0 {
		return nil
	}
	if NormalizeFileAllocation(mode) == FileAllocationPrealloc {
		return file.Truncate(size)
	}
	return preallocateFalloc(file, size)
}
