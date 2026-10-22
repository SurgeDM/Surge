package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/service"
)

func TestSettingsEditorPreservesConcurrentDaemonChanges(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
	settings := config.DefaultSettings()
	settings.Categories.Categories = nil
	mgr := orchestrator.NewLifecycleManager(nil, nil, settings)
	defer mgr.Shutdown()
	svc := service.NewLocalDownloadService(mgr)
	m := InitialRootModel(1701, "test", svc, mgr, settings, false)
	m.snapshotSettings()
	current := settings.Clone()
	current.Network.GlobalRateLimit.Value = "1024 B/s"
	if err := svc.UpdateSettings(current); err != nil {
		t.Fatal(err)
	}
	// Closing without editing must not write the stale snapshot.
	if err := m.persistSettings(); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.GetSettings()
	if got.Network.GlobalRateLimit.Value != "1024 B/s" {
		t.Fatal("unchanged close reverted daemon settings")
	}
	m.Settings.General.AutoResume.Value = !config.Resolve[bool](settings.General.AutoResume)
	if err := m.persistSettings(); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.GetSettings()
	if got.Network.GlobalRateLimit.Value != "1024 B/s" || got.General.AutoResume.Value == settings.General.AutoResume.Value {
		t.Fatal("save lost concurrent update or edit")
	}
	// Conflicting edits must stay in the editor and leave the owner unchanged.
	m.snapshotSettings()
	m.Settings.Network.MaxConcurrentDownloads.Value = 5
	current = got.Clone()
	current.Network.MaxConcurrentDownloads.Value = 6
	if err := svc.UpdateSettings(current); err != nil {
		t.Fatal(err)
	}
	if err := m.persistSettings(); !errors.Is(err, config.ErrSettingsConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	got, _ = svc.GetSettings()
	if got.Network.MaxConcurrentDownloads.Value != 6 {
		t.Fatal("conflict overwrote daemon setting")
	}
}

func TestOpeningSettingsRefreshesDaemonSnapshot(t *testing.T) {
	settings := config.DefaultSettings()
	mgr := orchestrator.NewLifecycleManager(nil, nil, settings)
	defer mgr.Shutdown()
	svc := service.NewLocalDownloadService(mgr)
	m := InitialRootModel(1701, "test", svc, mgr, settings, false)
	current := settings.Clone()
	current.Network.GlobalRateLimit.Value = "2048 B/s"
	mgr.ApplySettings(current)
	updated, _ := m.updateDashboard(tea.KeyPressMsg{Text: "s", Code: 's'})
	got := updated.(RootModel)
	if got.state != SettingsState || got.Settings.Network.GlobalRateLimit.Value != "2048 B/s" || got.SettingsBaseline.Network.GlobalRateLimit.Value != "2048 B/s" {
		t.Fatalf("editor did not refresh settings: state=%v", got.state)
	}
}
