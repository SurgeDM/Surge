package tui

import (
	"errors"
	"strings"
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
	// A filter with no matches must still show a failed save when closing.
	m.state = SettingsState
	m.width, m.height = 120, 40
	m.settingsSearchQuery = "no-setting-matches-this"
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(RootModel)
	if m.state != SettingsState || m.settingsError == "" {
		t.Fatal("failed save should keep settings open with an error")
	}
	view := stripANSI.ReplaceAllString(m.viewSettings(), "")
	if !strings.Contains(view, "No matching settings") || !strings.Contains(view, "Failed to save settings") {
		t.Fatalf("no-match view hid save failure: %s", view)
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

func TestReorderCategories(t *testing.T) {
	settings := config.DefaultSettings()
	settings.Categories.Categories = []config.Category{{Name: "First"}, {Name: "Second"}, {Name: "Third"}}
	m := RootModel{Settings: settings, keys: config.DefaultKeyMap(), SettingsFocusedPane: 1}
	for i, name := range config.CategoryOrder() {
		if name == "Categories" {
			m.SettingsActiveTab = i
		}
	}
	m.SettingsSelectedRow = len(config.GetSettingsMetadata()["Categories"]) + 1
	for _, tc := range []struct {
		key       tea.KeyPressMsg
		wantIndex int
	}{
		{tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}, 0},
		{tea.KeyPressMsg{Code: 'j', Text: "J", Mod: tea.ModShift}, 1},
		{tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}, 2},
		{tea.KeyPressMsg{Code: 'k', Text: "K", Mod: tea.ModShift}, 1},
	} {
		updated, _ := m.updateSettings(tc.key)
		m = updated.(RootModel)
		if m.Settings.Categories.Categories[tc.wantIndex].Name != "Second" || m.SettingsSelectedRow != len(config.GetSettingsMetadata()["Categories"])+tc.wantIndex {
			t.Fatalf("move %s: categories = %v, row = %d", tc.key.String(), m.Settings.Categories.Categories, m.SettingsSelectedRow)
		}
	}
	updated, _ := m.updateSettings(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	m = updated.(RootModel)
	if config.DiffSettings(m.SettingsBaseline, m.Settings).Categories == nil {
		t.Fatal("reordered categories not included in settings changes")
	}
}
