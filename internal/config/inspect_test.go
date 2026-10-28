package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInspectRejectsInvalidSettingsWithoutRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	for _, input := range []string{"[network", "[network]\nunknown_option=1", "[network]\nmax_connections_per_download='secret'", "[unknown]\nx=1"} {
		if err := os.WriteFile(path, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := InspectSettings(path); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	settings := DefaultSettings()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, _, err := InspectSettings(GetSettingsPath()); err != nil {
		t.Fatalf("cannot inspect application's own TOML: %v", err)
	}
}
