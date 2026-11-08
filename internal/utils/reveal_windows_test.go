//go:build windows

package utils

import (
	"path/filepath"
	"testing"
)

func TestRevealFile_MissingFileFails(t *testing.T) {
	// A path the shell cannot resolve must fail before any window opens, so
	// OpenContainingFolder falls back to opening the directory.
	if err := revealFile(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("expected an error for a file that does not exist")
	}
}
