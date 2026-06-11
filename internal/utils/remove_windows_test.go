package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoveFile_WindowsRetry_Success(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "testfile_retry.txt")

	// Create a file and hold it open to lock it
	f, err := os.Create(file)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Start a goroutine to close the file after a short delay
	// This will cause the first few Remove attempts to fail,
	// but an attempt after 200ms should succeed.
	go func() {
		time.Sleep(200 * time.Millisecond)
		f.Close()
	}()

	// Remove it (should block, retry, and eventually succeed)
	if err := RemoveFile(file); err != nil {
		t.Fatalf("RemoveFile failed despite retry: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("File still exists after RemoveFile")
	}
}

func TestRemoveFile_WindowsRetry_Exhausted(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "testfile_exhaust.txt")

	// Create a file and hold it open to lock it permanently
	f, err := os.Create(file)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	// Make sure we close it at the very end so we don't leak handles
	defer f.Close()

	// Remove it (should exhaust retries and fail)
	err = RemoveFile(file)
	if err == nil {
		t.Fatalf("RemoveFile succeeded unexpectedly while file was locked")
	}
}
