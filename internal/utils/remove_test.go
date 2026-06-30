package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveFile_Success(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "testfile.txt")

	// Create a file
	if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Remove it
	if err := RemoveFile(file); err != nil {
		t.Fatalf("RemoveFile failed: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("File still exists after RemoveFile")
	}
}

func TestRemoveFile_NotExist(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "nonexistent.txt")

	// Remove a file that doesn't exist
	if err := RemoveFile(file); err != nil {
		t.Fatalf("RemoveFile on non-existent file failed: %v", err)
	}
}
