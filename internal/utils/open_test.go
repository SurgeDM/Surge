package utils

import (
	"strings"
	"testing"
)

func TestOpenFile_Validation(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name       string
		path       string
		errorHints []string
	}{
		{
			name:       "empty path",
			path:       "",
			errorHints: []string{"empty"},
		},
		{
			name:       "directory path",
			path:       tempDir,
			errorHints: []string{"directory", "is a directory"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := OpenFile(tc.path)
			if err == nil {
				t.Fatalf("expected validation error for %q", tc.path)
			}

			lower := strings.ToLower(err.Error())
			for _, hint := range tc.errorHints {
				if strings.Contains(lower, strings.ToLower(hint)) {
					return
				}
			}

			t.Fatalf("expected error containing one of %v, got: %v", tc.errorHints, err)
		})
	}
}

func TestOpenContainingFolder_Validation(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		errorHints []string
	}{
		{
			name:       "empty path",
			path:       "",
			errorHints: []string{"empty"},
		},
		{
			name:       "dot path",
			path:       ".",
			errorHints: []string{"cannot resolve"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := OpenContainingFolder(tc.path)
			if err == nil {
				t.Fatalf("expected validation error for %q", tc.path)
			}

			lower := strings.ToLower(err.Error())
			for _, hint := range tc.errorHints {
				if strings.Contains(lower, strings.ToLower(hint)) {
					return
				}
			}

			t.Fatalf("expected error containing one of %v, got: %v", tc.errorHints, err)
		})
	}
}
