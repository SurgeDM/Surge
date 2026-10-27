package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

// These tests run the real binary: command hooks, package initialization, and
// environment handling must all respect the metadata/client startup boundary.
func TestCLIStartupBoundaries(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "surge")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test binary: %v\n%s", err, output)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/download" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer startup-test-token" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "remote-job-id"})
	}))
	defer server.Close()
	cases := [][]string{{"--help"}, {"--version"}, {"help", "server"}, {"server", "start", "--help"}, {"config", "help"}, {"completion", "bash"}, {"completion", "zsh"}, {"completion", "fish"}, {"completion", "powershell"}, {"__complete", "config", ""}, {"add", "--output-version=1", "https://example.com/file"}}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := t.TempDir()
			environment := append([]string(nil), os.Environ()...)
			for _, key := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR", "XDG_DATA_HOME", "XDG_CACHE_HOME", "APPDATA", "LOCALAPPDATA"} {
				// A file cannot contain Surge directories/settings. Metadata and explicit
				// remote commands should succeed without trying to initialize any of them.
				path := filepath.Join(root, key)
				if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
				environment = append(environment, key+"="+path)
			}
			environment = append(environment, "SURGE_HOST="+server.URL, "SURGE_TOKEN=startup-test-token")
			before := requests.Load()
			command := exec.Command(binary, args...)
			command.Env = environment
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("command failed: %v\n%s", err, output)
			}
			if len(output) == 0 {
				t.Fatal("command produced no output")
			}
			expected := int32(0)
			if args[0] == "add" {
				expected = 1
			}
			if calls := requests.Load() - before; calls != expected {
				t.Fatalf("network calls=%d want=%d", calls, expected)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 7 {
				t.Fatalf("unexpected local setup output: %v", entries)
			}
			for _, entry := range entries {
				data, err := os.ReadFile(filepath.Join(root, entry.Name()))
				if err != nil || string(data) != "unchanged" {
					t.Fatalf("local startup path modified: %s (%v)", entry.Name(), err)
				}
			}
		})
	}
}
