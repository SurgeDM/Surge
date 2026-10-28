package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/spf13/cobra"
)

func TestDoctorDoesNotCreateOrRepairFiles(t *testing.T) {
	setupIsolatedCmdState(t)
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(root, "runtime"))
	report := localDoctor()
	if !report.OK {
		t.Fatalf("fresh environment unhealthy: %+v", report)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("doctor created files: %v %v", entries, err)
	}
	if err := os.MkdirAll(config.GetSurgeDir(), 0700); err != nil {
		t.Fatal(err)
	}
	path := config.GetSettingsPath()
	input := []byte("[network\ninvalid=secret")
	if err := os.WriteFile(path, input, 0600); err != nil {
		t.Fatal(err)
	}
	if report := localDoctor(); report.OK {
		t.Fatal("corrupt settings accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(input, after) {
		t.Fatal("doctor repaired corrupt settings")
	}
}
func TestConfigInspectionRedactsSecrets(t *testing.T) {
	setupIsolatedCmdState(t)
	resetCommandConnectionState(t)
	if err := os.MkdirAll(config.GetSurgeDir(), 0700); err != nil {
		t.Fatal(err)
	}
	input := "[network]\nproxy_url = 'http://user:private-password@example.com:3128/path?token=private-query'\n[extension]\nauth_token = 'private-auth'\n"
	if err := os.WriteFile(config.GetSettingsPath(), []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "surge", SilenceErrors: true, SilenceUsage: true}
	root.AddCommand(newConfigInspectCommand("explain"))
	var out, diagnostic bytes.Buffer
	if code := runCommand(root, []string{"explain", "--json"}, &out, &diagnostic); code != 0 {
		t.Fatalf("code=%d %s", code, diagnostic.String())
	}
	for _, secret := range []string{"private-password", "private-query", "private-auth"} {
		if strings.Contains(out.String(), secret) {
			t.Fatal("secret leaked")
		}
	}
	if !strings.Contains(out.String(), `"source": "file"`) {
		t.Fatal("source missing")
	}
}
