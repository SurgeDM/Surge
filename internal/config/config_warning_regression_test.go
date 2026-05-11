package config

// Regression tests for: config problems must be surfaced in StartupWarnings,
// never silently swallowed.
//
// Root causes fixed (branch fix-config-fails):
//  1. Corrupt settings.json returned DefaultSettings() with no warning at all.
//  2. publishStartupWarnings() fired before the TUI event stream was connected,
//     so valid warnings were silently dropped.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadSettings_CorruptJSON_PopulatesStartupWarning is the primary regression
// test for bug 2: corrupt settings must set StartupWarnings, not return silent defaults.
func TestLoadSettings_CorruptJSON_PopulatesStartupWarning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	surgeDir := GetSurgeDir()
	if err := os.MkdirAll(surgeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(surgeDir, "settings.json"), []byte("{not valid json!!!"), 0o644); err != nil {
		t.Fatalf("write corrupt settings: %v", err)
	}

	settings, err := LoadSettings()

	if err != nil {
		t.Fatalf("LoadSettings must not return an error for corrupt JSON, got: %v", err)
	}
	if settings == nil {
		t.Fatal("LoadSettings returned nil settings for corrupt JSON")
	}

	// THE REGRESSION: StartupWarnings must NOT be empty for a corrupt file.
	if len(settings.StartupWarnings) == 0 {
		t.Fatal("corrupt settings.json produced no StartupWarnings — config problems would be silently hidden")
	}

	// The warning should mention both the corruption and the reset action.
	warn := strings.Join(settings.StartupWarnings, " ")
	if !strings.Contains(strings.ToLower(warn), "corrupt") {
		t.Errorf("warning should mention 'corrupt', got: %q", warn)
	}
	if !strings.Contains(strings.ToLower(warn), "reset") && !strings.Contains(strings.ToLower(warn), "default") {
		t.Errorf("warning should mention 'reset' or 'default', got: %q", warn)
	}
}

// TestLoadSettings_TruncatedJSON_PopulatesStartupWarning covers the crash-during-write
// scenario (atomically incomplete file).
func TestLoadSettings_TruncatedJSON_PopulatesStartupWarning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	surgeDir := GetSurgeDir()
	if err := os.MkdirAll(surgeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	truncated := `{"general": {"default_download_dir": "/home/user/Downloads", "warn_on_duplicate": tr`
	if err := os.WriteFile(filepath.Join(surgeDir, "settings.json"), []byte(truncated), 0o644); err != nil {
		t.Fatalf("write truncated settings: %v", err)
	}

	settings, err := LoadSettings()

	if err != nil {
		t.Fatalf("LoadSettings must not return an error for truncated JSON, got: %v", err)
	}
	if settings == nil {
		t.Fatal("LoadSettings returned nil for truncated JSON")
	}
	if len(settings.StartupWarnings) == 0 {
		t.Fatal("truncated settings.json produced no StartupWarnings — config problems would be silently hidden")
	}
}

// TestLoadSettings_ValidSettings_NoStartupWarnings ensures clean configs stay quiet.
func TestLoadSettings_ValidSettings_NoStartupWarnings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	defaults := DefaultSettings()
	if err := SaveSettings(defaults); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	settings, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings == nil {
		t.Fatal("LoadSettings returned nil for valid settings")
	}
	if len(settings.StartupWarnings) != 0 {
		t.Errorf("valid settings should produce zero StartupWarnings, got: %v", settings.StartupWarnings)
	}
}

// TestLoadSettings_MissingFile_NoStartupWarnings covers the first-run case where
// no settings file exists — this is expected and must not produce warnings.
func TestLoadSettings_MissingFile_NoStartupWarnings(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// No file created — GetSurgeDir() path doesn't exist, settings.json absent.

	settings, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if settings == nil {
		t.Fatal("LoadSettings returned nil for missing file")
	}
	if len(settings.StartupWarnings) != 0 {
		t.Errorf("missing settings file should not produce warnings (first run), got: %v", settings.StartupWarnings)
	}
}

// TestValidate_InvalidField_PopulatesStartupWarnings ensures that field-level
// validation warnings (out-of-range values, invalid paths) also surface.
func TestValidate_InvalidField_PopulatesStartupWarnings(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Settings)
	}{
		{
			name:   "MaxConnectionsPerHost out of range",
			mutate: func(s *Settings) { s.Network.MaxConnectionsPerHost = 999 },
		},
		{
			name:   "MaxConcurrentDownloads out of range",
			mutate: func(s *Settings) { s.Network.MaxConcurrentDownloads = 99 },
		},
		{
			name:   "MaxTaskRetries out of range",
			mutate: func(s *Settings) { s.Performance.MaxTaskRetries = 999 },
		},
		{
			name:   "SlowWorkerThreshold out of range",
			mutate: func(s *Settings) { s.Performance.SlowWorkerThreshold = 5.0 },
		},
		{
			name:   "LogRetentionCount out of range",
			mutate: func(s *Settings) { s.General.LogRetentionCount = 0 },
		},
		{
			name:   "Invalid proxy URL",
			mutate: func(s *Settings) { s.Network.ProxyURL = "not-a-url" },
		},
		{
			name:   "Invalid DNS server",
			mutate: func(s *Settings) { s.Network.CustomDNS = "not.a.valid.ip.server.!!!" },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := DefaultSettings()
			tc.mutate(s)
			s.Validate()

			if len(s.StartupWarnings) == 0 {
				t.Errorf("expected at least one StartupWarning for invalid field, got none")
			}
		})
	}
}

// TestValidate_MultipleInvalidFields_AllWarningsPresent ensures each invalid
// field independently contributes a warning (no short-circuiting).
func TestValidate_MultipleInvalidFields_AllWarningsPresent(t *testing.T) {
	s := DefaultSettings()
	s.Network.MaxConnectionsPerHost = 999  // invalid
	s.Network.MaxConcurrentDownloads = 99  // invalid
	s.Performance.SlowWorkerThreshold = -1 // invalid
	s.Validate()

	if len(s.StartupWarnings) < 3 {
		t.Errorf("expected at least 3 warnings for 3 invalid fields, got %d: %v",
			len(s.StartupWarnings), s.StartupWarnings)
	}
}

// TestValidate_ClearsOldWarningsOnRevalidation ensures Validate() is idempotent:
// it starts fresh each call (sets StartupWarnings = nil first), so a second call
// on already-reset settings produces zero warnings rather than accumulating.
func TestValidate_ClearsOldWarningsOnRevalidation(t *testing.T) {
	s := DefaultSettings()
	s.Network.MaxConnectionsPerHost = 999 // invalid — will be reset to default
	s.Validate()

	firstCount := len(s.StartupWarnings)
	if firstCount == 0 {
		t.Fatal("expected at least one warning on first Validate()")
	}

	// After Validate(), MaxConnectionsPerHost has been reset to the default (valid).
	// A second Validate() should find nothing wrong and produce zero warnings.
	// This confirms that warnings are cleared and not accumulated across calls.
	s.Validate()
	secondCount := len(s.StartupWarnings)

	if secondCount != 0 {
		t.Errorf("second Validate() on already-reset settings should produce 0 warnings, got %d: %v",
			secondCount, s.StartupWarnings)
	}
}

// TestLoadSettings_CorruptJSON_ReturnsDefaultValues verifies that the returned
// settings are actually defaults, not a partially-parsed struct.
func TestLoadSettings_CorruptJSON_ReturnsDefaultValues(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	surgeDir := GetSurgeDir()
	if err := os.MkdirAll(surgeDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(surgeDir, "settings.json"), []byte("GARBAGE"), 0o644); err != nil {
		t.Fatalf("write corrupt settings: %v", err)
	}

	settings, _ := LoadSettings()
	defaults := DefaultSettings()

	if settings == nil {
		t.Fatal("LoadSettings returned nil")
	}
	if settings.Network.MaxConnectionsPerHost != defaults.Network.MaxConnectionsPerHost {
		t.Errorf("MaxConnectionsPerHost = %d, want default %d",
			settings.Network.MaxConnectionsPerHost, defaults.Network.MaxConnectionsPerHost)
	}
	if settings.Performance.MaxTaskRetries != defaults.Performance.MaxTaskRetries {
		t.Errorf("MaxTaskRetries = %d, want default %d",
			settings.Performance.MaxTaskRetries, defaults.Performance.MaxTaskRetries)
	}
}
