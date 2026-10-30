package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func TestSettingsSearchEditsFilteredSetting(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, nil, config.DefaultSettings(), false)
	m.state = SettingsState
	m.width, m.height = 120, 40
	original := config.Resolve[bool](m.Settings.Network.SequentialDownload)
	press := func(msg tea.KeyPressMsg) { t.Helper(); updated, _ := m.Update(msg); m = updated.(RootModel) }
	press(tea.KeyPressMsg{Code: '/', Text: "/"})
	updated, _ := m.Update(tea.PasteMsg{Content: "sequential_download"})
	m = updated.(RootModel)
	if m.currentSettingsCategory() != "Downloads" || m.getSettingsCount() != 1 || m.getCurrentSettingKey() != "sequential_download" {
		t.Fatalf("wrong filtered setting: %s", m.getCurrentSettingKey())
	}
	press(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.settingsSearchActive || m.SettingsIsEditing {
		t.Fatal("search confirm should only commit filter")
	}
	if !strings.Contains(m.viewSettings(), "[enter] Toggle") {
		t.Fatal("missing toggle hint")
	}
	press(tea.KeyPressMsg{Code: tea.KeyEnter})
	if config.Resolve[bool](m.Settings.Network.SequentialDownload) == original {
		t.Fatal("Enter did not toggle filtered setting")
	}
	press(tea.KeyPressMsg{Code: 'f', Text: "f"})
	press(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.settingsSearchQuery != "" || m.settingsSearchActive || m.state != SettingsState {
		t.Fatal("Ctrl+C did not clear settings search")
	}
}

func TestSettingsSearchNoMatchesStillShowsSearch(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, nil, config.DefaultSettings(), false)
	m.keys = config.DefaultKeyMap() // Keep the hint independent of the saved keymap.
	m.width, m.height = 120, 40
	m.settingsSearchQuery = "no-setting-matches-this"
	text := stripANSI.ReplaceAllString(m.viewSettings(), "")
	if !strings.Contains(text, "No matching settings") || !strings.Contains(text, "[f /] Edit") {
		t.Fatalf("missing empty search feedback: %s", text)
	}
}

func TestContextHintsUseConfiguredKeys(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, nil, config.DefaultSettings(), false)
	m.keys.ApplyConfig(&config.KeyMapConfig{Dashboard: map[string]config.KeyBindingConfig{
		"Log":     {Keys: []string{"ctrl+l"}},
		"CopyURL": {Keys: []string{"ctrl+y"}},
		"CopyID":  {Keys: []string{"ctrl+i"}},
	}})
	if !strings.Contains(m.renderLogBox(70, 10), "[ctrl+l]") {
		t.Fatal("log hint ignores configured binding")
	}

}

func TestGlobalSettingsSearchChangesValueInOriginalCategory(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, nil, config.DefaultSettings(), false)
	m.state = SettingsState
	m.width, m.height = 120, 40
	m.settingsSearchQuery = "global_rate_limit"
	m.settingsSearchInput.SetValue(m.settingsSearchQuery)
	m.SettingsFocusedPane = 1
	if m.getCurrentSettingKey() != "global_rate_limit" || m.currentSettingsCategory() != "Downloads" {
		t.Fatal("search did not find setting outside current category")
	}
	if !strings.Contains(m.viewSettings(), "[enter] Change value") {
		t.Fatal("missing change-value hint")
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(RootModel)
	if !m.SettingsIsEditing {
		t.Fatal("Enter did not edit filtered result")
	}
	m.SettingsInput.SetValue("10")
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(RootModel)
	if m.SettingsIsEditing || m.settingsError != "" {
		t.Fatalf("failed to save result: %s", m.settingsError)
	}
	if m.Settings.Network.GlobalRateLimit.Value != "10 MiB/s" {
		t.Fatalf("wrong saved value: %v", m.Settings.Network.GlobalRateLimit.Value)
	}
}

func TestSettingsSearchClearedOnCloseAndReopen(t *testing.T) {
	m := confirmationSettingsModel(t, "General", "clipboard_monitor")
	m.SaveSettingsFunc = func(*config.Settings) error { return nil }
	m.settingsSearchQuery = "clipboard_monitor"
	m.settingsSearchInput.SetValue(m.settingsSearchQuery)
	m = settingsPress(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.state != DashboardState || m.settingsSearchQuery != "" || m.settingsSearchInput.Value() != "" || m.settingsSearchActive {
		t.Fatal("closing settings retained search")
	}
	// Also cover entry with a stale filter, independently of the close path.
	m.settingsSearchQuery = "old filter"
	m.settingsSearchInput.SetValue(m.settingsSearchQuery)
	m = settingsPress(t, m, settingRune('s'))
	if m.state != SettingsState || m.settingsSearchQuery != "" || m.settingsSearchInput.Value() != "" || m.settingsSearchActive {
		t.Fatal("reopening settings retained search")
	}
}
