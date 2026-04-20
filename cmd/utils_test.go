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
	originalInsecureHTTP := globalInsecureHTTP
	defer func() {
		if err := os.Setenv("SURGE_HOST", originalHost); err != nil {
			t.Errorf("failed to restore environment variable: %v", err)
		}
		globalHost = originalGlobalHost
		globalInsecureHTTP = originalInsecureHTTP
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
				if err := os.Setenv("SURGE_HOST", "127.0.0.1:1234"); err != nil {
					t.Fatalf("failed to set environment variable: %v", err)
				}
				globalHost = ""
			},
			outputDir: "",
			wantExact: "",
		},
		{
			name: "Remote Host Set via Global - Pass Through Exact",
			setupHost: func() {
				if err := os.Setenv("SURGE_HOST", ""); err != nil {
					t.Fatalf("failed to set environment variable: %v", err)
				}
				globalHost = "127.0.0.1:1234"
			},
			outputDir: ".",
			wantExact: ".",
		},
		{
			name: "Local Execution - Empty Dir returns CWD",
			setupHost: func() {
				if err := os.Setenv("SURGE_HOST", ""); err != nil {
					t.Fatalf("failed to set environment variable: %v", err)
				}
				globalHost = ""
			},
			outputDir: "",
			wantExact: wd,
		},
		{
			name: "Local Execution - Dot returns Absolute CWD",
			setupHost: func() {
				if err := os.Setenv("SURGE_HOST", ""); err != nil {
					t.Fatalf("failed to set environment variable: %v", err)
				}
				globalHost = ""
			},
			outputDir: ".",
			wantExact: wd,
		},
		{
			name: "Local Execution - Relative Subdir returns Absolute",
			setupHost: func() {
				if err := os.Setenv("SURGE_HOST", ""); err != nil {
					t.Fatalf("failed to set environment variable: %v", err)
				}
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

func TestResolveAPIConnection_UsesSharedInsecureHTTPSetting(t *testing.T) {
	originalGlobalHost := globalHost
	originalGlobalToken := globalToken
	originalInsecureHTTP := globalInsecureHTTP
	defer func() {
		globalHost = originalGlobalHost
		globalToken = originalGlobalToken
		globalInsecureHTTP = originalInsecureHTTP
	}()

	globalHost = "http://example.com:1700"
	globalToken = "test-token"
	globalInsecureHTTP = false

	if _, _, err := resolveAPIConnection(true); err == nil {
		t.Fatal("expected insecure HTTP target to be rejected when insecure-http is disabled")
	} else if !strings.Contains(err.Error(), "--insecure-http") {
		t.Fatalf("expected insecure HTTP error, got: %v", err)
	}

	globalInsecureHTTP = true

	baseURL, _, err := resolveAPIConnection(true)
	if err != nil {
		t.Fatalf("resolveAPIConnection returned error with insecure-http enabled: %v", err)
	}
	if baseURL != "http://example.com:1700" {
		t.Fatalf("resolveAPIConnection baseURL = %q, want %q", baseURL, "http://example.com:1700")
	}
}
