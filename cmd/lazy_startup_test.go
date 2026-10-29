package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/SurgeDM/Surge/internal/testutil"
	"github.com/spf13/cobra"
)

func TestRemoteClientDoesNotResetOrCreateLocalEngine(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	previousPool, previousService := GlobalPool, GlobalService
	sentinel := &countingLifecycleService{}
	GlobalService = sentinel
	GlobalPool = nil
	t.Cleanup(func() {
		if GlobalPool != nil {
			GlobalPool.GracefulShutdown()
		}
		GlobalPool, GlobalService = previousPool, previousService
	})
	server := testutil.NewHTTPServerT(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "remote-job-id"})
	}))
	defer server.Close()
	globalHost, globalToken = server.URL, "test-token"
	// Use a fresh command tree to avoid retaining parsed global flags between tests.
	root := &cobra.Command{Use: "surge", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newAddCommand())
	var out, diagnostic bytes.Buffer
	if code := runCommand(root, []string{"add", "--output-version=1", "https://example.com/file"}, &out, &diagnostic); code != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, diagnostic.String(), out.String())
	}
	if GlobalPool != nil || GlobalService != sentinel {
		t.Fatal("remote client created or reset a local runtime")
	}
}

func TestCompletionDoesNotResetLocalEngine(t *testing.T) {
	if rootCmd.PersistentPreRun != nil || rootCmd.PersistentPreRunE != nil {
		t.Fatal("root command must not define inherited startup hooks")
	}
	setupIsolatedCmdState(t)
	previousService, previousPool := GlobalService, GlobalPool
	sentinel := &countingLifecycleService{}
	GlobalService, GlobalPool = sentinel, nil
	t.Cleanup(func() {
		if GlobalPool != nil {
			GlobalPool.GracefulShutdown()
		}
		GlobalService, GlobalPool = previousService, previousPool
	})
	root := &cobra.Command{Use: "surge"}
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"completion", "bash"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("completion script is empty")
	}
	if GlobalService != sentinel || GlobalPool != nil {
		t.Fatal("completion reset a local runtime")
	}
}
