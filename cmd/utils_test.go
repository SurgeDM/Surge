package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveClientOutputPath(t *testing.T) {
	// Save original env vars to restore later
	originalHost := os.Getenv("SURGE_HOST")
	originalGlobalHost := globalHost
	defer func() {
		os.Setenv("SURGE_HOST", originalHost)
		globalHost = originalGlobalHost
	}()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	tests := []struct {
		name       string
		setupHost  func()
		outputDir  string
		wantPrefix string // Used for absolute paths where exact value depends on OS/CWD
		wantExact  string
	}{
		{
			name: "Remote Host Set via Env - Pass Through Empty",
			setupHost: func() {
				os.Setenv("SURGE_HOST", "127.0.0.1:1234")
				globalHost = ""
			},
			outputDir: "",
			wantExact: "",
		},
		{
			name: "Remote Host Set via Global - Pass Through Exact",
			setupHost: func() {
				os.Setenv("SURGE_HOST", "")
				globalHost = "127.0.0.1:1234"
			},
			outputDir: ".",
			wantExact: ".",
		},
		{
			name: "Local Execution - Empty Dir returns CWD",
			setupHost: func() {
				os.Setenv("SURGE_HOST", "")
				globalHost = ""
			},
			outputDir: "",
			wantExact: wd,
		},
		{
			name: "Local Execution - Dot returns Absolute CWD",
			setupHost: func() {
				os.Setenv("SURGE_HOST", "")
				globalHost = ""
			},
			outputDir: ".",
			wantExact: wd,
		},
		{
			name: "Local Execution - Relative Subdir returns Absolute",
			setupHost: func() {
				os.Setenv("SURGE_HOST", "")
				globalHost = ""
			},
			outputDir: "downloads",
			wantExact: filepath.Join(wd, "downloads"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupHost()
			got := resolveClientOutputPath(tt.outputDir)

			if got != tt.wantExact {
				t.Errorf("resolveClientOutputPath(%q) = %q, want exactly %q", tt.outputDir, got, tt.wantExact)
			}
			if tt.wantPrefix != "" {
				rel, err := filepath.Rel(tt.wantPrefix, got)
				if err != nil || strings.HasPrefix(rel, "..") {
					t.Errorf("resolveClientOutputPath(%q) = %q, want prefix %q", tt.outputDir, got, tt.wantPrefix)
				}
			}
		})
	}
}
