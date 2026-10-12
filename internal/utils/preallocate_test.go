package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPreallocateFile(t *testing.T) {
	for _, mode := range []string{FileAllocationFalloc, FileAllocationPrealloc, "", "FALLOC"} {
		t.Run(fmt.Sprintf("%q", mode), func(t *testing.T) {
			f := openPreallocFile(t)
			const size = int64(8192)
			if err := PreallocateFile(f, size, mode); err != nil {
				t.Fatalf("PreallocateFile: %v", err)
			}
			info, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() != size {
				t.Fatalf("size = %d, want %d", info.Size(), size)
			}
		})
	}

	t.Run("non-positive size", func(t *testing.T) {
		f := openPreallocFile(t)
		if _, err := f.Write([]byte("abc")); err != nil {
			t.Fatal(err)
		}
		for _, size := range []int64{0, -1} {
			if err := PreallocateFile(f, size, FileAllocationFalloc); err != nil {
				t.Fatalf("size %d: %v", size, err)
			}
			info, err := f.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() != 3 {
				t.Fatalf("PreallocateFile(%d) left size %d, want 3", size, info.Size())
			}
		}
	})
}

func TestPreallocateFile_LinuxFallocate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux fallocate")
	}

	t.Run("syscall", func(t *testing.T) {
		f := openPreallocFile(t)
		const size = int64(64 * 1024)
		if err := PreallocateFile(f, size, FileAllocationFalloc); err != nil {
			t.Fatalf("PreallocateFile: %v", err)
		}
		assertFileSize(t, f, size)
	})

	t.Run("fallback", func(t *testing.T) {
		called := false
		restore := setFallocateForTest(func(int, int64) error {
			called = true
			return os.ErrInvalid
		})
		defer restore()

		f := openPreallocFile(t)
		const size = int64(4096)
		if err := PreallocateFile(f, size, FileAllocationFalloc); err != nil {
			t.Fatalf("fallback: %v", err)
		}
		if !called {
			t.Fatal("fallocate was not attempted")
		}
		assertFileSize(t, f, size)
	})
}

func openPreallocFile(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "prealloc.bin"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func assertFileSize(t *testing.T, f *os.File, size int64) {
	t.Helper()
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != size {
		t.Fatalf("size = %d, want %d", info.Size(), size)
	}
}
