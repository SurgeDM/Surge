//go:build windows

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPreallocateFile_WindowsSparse(t *testing.T) {
	dir := t.TempDir()

	fallocPath := filepath.Join(dir, "falloc.bin")
	falloc := createPrealloc(t, fallocPath)
	if err := PreallocateFile(falloc, 8192, FileAllocationFalloc); err != nil {
		t.Fatalf("falloc: %v", err)
	}
	if !isSparseFile(t, fallocPath) {
		t.Fatal("falloc did not set FILE_ATTRIBUTE_SPARSE_FILE")
	}

	preallocPath := filepath.Join(dir, "prealloc.bin")
	prealloc := createPrealloc(t, preallocPath)
	if err := PreallocateFile(prealloc, 8192, FileAllocationPrealloc); err != nil {
		t.Fatalf("prealloc: %v", err)
	}
	if isSparseFile(t, preallocPath) {
		t.Fatal("prealloc set FILE_ATTRIBUTE_SPARSE_FILE")
	}
}

func createPrealloc(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func isSparseFile(t *testing.T, path string) bool {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		t.Fatal(err)
	}
	return attrs&windows.FILE_ATTRIBUTE_SPARSE_FILE != 0
}
