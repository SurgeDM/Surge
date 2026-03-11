package runtime

import (
	"path/filepath"
	"testing"

	"github.com/surge-downloader/surge/internal/config"
)

func TestLoadSettingsOrDefault_ReturnsDefaultsWhenMissing(t *testing.T) {
	setupRuntimeTestEnv(t)

	settings := LoadSettingsOrDefault()
	if settings == nil {
		t.Fatal("expected default settings")
	}
	if settings.Network.MaxConcurrentDownloads <= 0 {
		t.Fatalf("expected defaults to be populated, got %+v", settings.Network)
	}
}

func TestAppSettings_ReloadAndApply(t *testing.T) {
	setupRuntimeTestEnv(t)

	app := NewEmpty()

	initial := config.DefaultSettings()
	initial.General.DefaultDownloadDir = filepath.Join(t.TempDir(), "downloads-initial")
	if err := config.SaveSettings(initial); err != nil {
		t.Fatalf("SaveSettings(initial) failed: %v", err)
	}

	if got := app.Settings(); got.General.DefaultDownloadDir != initial.General.DefaultDownloadDir {
		t.Fatalf("Settings() default dir = %q, want %q", got.General.DefaultDownloadDir, initial.General.DefaultDownloadDir)
	}

	updated := config.DefaultSettings()
	updated.General.DefaultDownloadDir = filepath.Join(t.TempDir(), "downloads-updated")
	if err := config.SaveSettings(updated); err != nil {
		t.Fatalf("SaveSettings(updated) failed: %v", err)
	}

	if got := app.Settings(); got.General.DefaultDownloadDir != initial.General.DefaultDownloadDir {
		t.Fatalf("Settings() should return cached settings, got %q want %q", got.General.DefaultDownloadDir, initial.General.DefaultDownloadDir)
	}

	if got := app.ReloadSettings(); got.General.DefaultDownloadDir != updated.General.DefaultDownloadDir {
		t.Fatalf("ReloadSettings() default dir = %q, want %q", got.General.DefaultDownloadDir, updated.General.DefaultDownloadDir)
	}

	override := config.DefaultSettings()
	override.General.DefaultDownloadDir = filepath.Join(t.TempDir(), "downloads-override")
	app.ApplySettings(override)

	if got := app.Settings(); got != override {
		t.Fatal("expected ApplySettings to replace cached settings")
	}
}

func TestAppApplySettings_NilUsesDefaults(t *testing.T) {
	app := NewEmpty()

	app.ApplySettings(nil)

	settings := app.Settings()
	if settings == nil {
		t.Fatal("expected default settings after ApplySettings(nil)")
	}
	if settings.Network.MaxConcurrentDownloads <= 0 {
		t.Fatalf("expected default settings values, got %+v", settings.Network)
	}
}
