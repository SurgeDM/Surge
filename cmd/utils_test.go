package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
)

func TestParseAndNormalizeURLArg(t *testing.T) {
	tests := []struct {
		name        string
		arg         string
		wantURL     string
		wantMirrors []string
		wantErr     bool
	}{
		{
			name:        "normalizes bare primary URL",
			arg:         "example.com/file.zip",
			wantURL:     "https://example.com/file.zip",
			wantMirrors: []string{"https://example.com/file.zip"},
		},
		{
			name:        "normalizes primary URL with mirrors",
			arg:         "example.com/file.zip,https://mirror.example.com/file.zip",
			wantURL:     "https://example.com/file.zip",
			wantMirrors: []string{"https://example.com/file.zip", "https://mirror.example.com/file.zip"},
		},
		{name: "rejects unsupported scheme", arg: "ftp://example.com/file.zip", wantErr: true},
		{name: "rejects unsupported mirror scheme", arg: "https://example.com/file.zip,FTP://mirror.example.com/file.zip", wantErr: true},
		{
			name:        "preserves commas in query values",
			arg:         "https://archive.org/compress/item/formats=PNG,ITEM TILE,LOG,HTTPS://mirror.example.com/item.zip",
			wantURL:     "https://archive.org/compress/item/formats=PNG,ITEM TILE,LOG",
			wantMirrors: []string{"https://archive.org/compress/item/formats=PNG,ITEM TILE,LOG", "HTTPS://mirror.example.com/item.zip"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, mirrors, err := parseAndNormalizeURLArg(tt.arg)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAndNormalizeURLArg() error = %v", err)
			}
			if url != tt.wantURL {
				t.Fatalf("URL = %q, want %q", url, tt.wantURL)
			}
			if !reflect.DeepEqual(mirrors, tt.wantMirrors) {
				t.Fatalf("mirrors = %v, want %v", mirrors, tt.wantMirrors)
			}
		})
	}
}

func TestResolveClientOutputPath(t *testing.T) {
	// Save original env vars to restore later
	originalGlobalHost := globalHost
	originalInsecureHTTP := globalInsecureHTTP
	defer func() {
		globalHost = originalGlobalHost
		globalInsecureHTTP = originalInsecureHTTP
	}()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}

	tests := []struct {
		name       string
		setupHost  func(t *testing.T)
		outputDir  string
		wantPrefix string // Used for absolute paths where exact value depends on OS/CWD
		wantExact  string
	}{
		{
			name: "Remote Host Set via Env - Pass Through Empty",
			setupHost: func(t *testing.T) {
				t.Setenv("SURGE_HOST", "127.0.0.1:1234")
				globalHost = ""
			},
			outputDir: "",
			wantExact: "",
		},
		{
			name: "Remote Host Set via Global - Pass Through Exact",
			setupHost: func(t *testing.T) {
				t.Setenv("SURGE_HOST", "")
				globalHost = "127.0.0.1:1234"
			},
			outputDir: ".",
			wantExact: ".",
		},
		{
			name: "Local Execution - Empty Dir returns CWD",
			setupHost: func(t *testing.T) {
				t.Setenv("SURGE_HOST", "")
				globalHost = ""
			},
			outputDir: "",
			wantExact: wd,
		},
		{
			name: "Local Execution - Dot returns Absolute CWD",
			setupHost: func(t *testing.T) {
				t.Setenv("SURGE_HOST", "")
				globalHost = ""
			},
			outputDir: ".",
			wantExact: wd,
		},
		{
			name: "Local Execution - Relative Subdir returns Absolute",
			setupHost: func(t *testing.T) {
				t.Setenv("SURGE_HOST", "")
				globalHost = ""
			},
			outputDir: "downloads",
			wantExact: filepath.Join(wd, "downloads"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupHost(t)
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

func TestResolveAPIConnection_PairsLocalPortAndTokenFromSameState(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("APPDATA", tmpDir)
	t.Setenv("XDG_STATE_HOME", tmpDir)
	t.Setenv("SURGE_TOKEN", "")

	originalGlobalHost := globalHost
	originalGlobalToken := globalToken
	defer func() {
		globalHost = originalGlobalHost
		globalToken = originalGlobalToken
	}()
	globalHost = ""
	globalToken = ""

	if err := config.EnsureDirs(); err != nil {
		t.Fatalf("config.EnsureDirs() failed: %v", err)
	}

	t.Setenv("SURGE_SYSTEM_RUNTIME_DIR", config.GetRuntimeDir())
	t.Setenv("SURGE_SYSTEM_STATE_DIR", config.GetStateDir())
	if err := writeTokenToFile(filepath.Join(config.GetStateDir(), "token"), "paired-token"); err != nil {
		t.Fatalf("write token failed: %v", err)
	}
	saveActivePort(1777)
	defer removeActivePort()

	baseURL, token, err := resolveAPIConnection(true)
	if err != nil {
		t.Fatalf("resolveAPIConnection() returned error: %v", err)
	}
	if baseURL != "http://127.0.0.1:1777" {
		t.Fatalf("baseURL = %q, want %q", baseURL, "http://127.0.0.1:1777")
	}
	if token != "paired-token" {
		t.Fatalf("token = %q, want %q", token, "paired-token")
	}
}
