package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func TestSettingsNavigation_VimStylePaneTransitions(t *testing.T) {
	// 1. Initialize RootModel with default keymap and settings
	keys := config.DefaultKeyMap()
	settings := config.DefaultSettings()

	m := RootModel{
		state:               SettingsState,
		keys:                keys,
		Settings:            settings,
		SettingsActiveTab:   0,
		SettingsSelectedRow: 0,
		SettingsFocusedPane: 1, // Start with List focused
	}

	// 2. Press "k" (Up) at row 0 -> should wrap around to the last row
	upMsg := tea.KeyPressMsg{Code: 'k', Text: "k"}
	updated, _ := m.Update(upMsg)
	m = updated.(RootModel)

	expectedLastRow := m.getSettingsCount() - 1
	if m.SettingsFocusedPane != 1 {
		t.Errorf("Expected focus to remain on List (1) when pressing Up on first row, got %d", m.SettingsFocusedPane)
	}
	if m.SettingsSelectedRow != expectedLastRow {
		t.Errorf("Expected selected row to wrap to %d, got %d", expectedLastRow, m.SettingsSelectedRow)
	}

	// 2b. Press "j" (Down) at last row -> should wrap around to row 0
	downMsg := tea.KeyPressMsg{Code: 'j', Text: "j"}
	updated, _ = m.Update(downMsg)
	m = updated.(RootModel)

	if m.SettingsSelectedRow != 0 {
		t.Errorf("Expected selected row to wrap to 0, got %d", m.SettingsSelectedRow)
	}

	// 3. Press "l" (NextTab/right) while focused on List -> should shift active tab to 1
	rightMsg := tea.KeyPressMsg{Code: 'l', Text: "l"}
	updated, _ = m.Update(rightMsg)
	m = updated.(RootModel)

	if m.SettingsActiveTab != 1 {
		t.Errorf("Expected active tab to shift to 1 when pressing NextTab, got %d", m.SettingsActiveTab)
	}

	// 4. Press "j" (Down) while focused on List -> should move selection to row 1
	downMsg = tea.KeyPressMsg{Code: 'j', Text: "j"}
	updated, _ = m.Update(downMsg)
	m = updated.(RootModel)

	if m.SettingsSelectedRow != 1 {
		t.Errorf("Expected selected row to move to 1, got %d", m.SettingsSelectedRow)
	}
	if m.SettingsFocusedPane != 1 {
		t.Errorf("Expected focus to remain on List (1), got %d", m.SettingsFocusedPane)
	}

	// 6. Press "h" (PrevTab/left) while focused on List -> should change tab back to 0
	leftMsg := tea.KeyPressMsg{Code: 'h', Text: "h"}
	updated, _ = m.Update(leftMsg)
	m = updated.(RootModel)

	if m.SettingsActiveTab != 0 {
		t.Errorf("Expected tab to switch to 0, got %d", m.SettingsActiveTab)
	}

}

func TestSettingsNavigation_ResetAndBrowseGuards(t *testing.T) {
	keys := config.DefaultKeyMap()
	settings := config.DefaultSettings()

	m := RootModel{
		state:               SettingsState,
		keys:                keys,
		Settings:            settings,
		SettingsActiveTab:   0,
		SettingsSelectedRow: 0,
		SettingsFocusedPane: 0, // Tabs focused
	}

	// Verify "r" (Reset) is ignored when tabs are focused
	resetMsg := tea.KeyPressMsg{Code: 'r', Text: "r"}
	updated, _ := m.Update(resetMsg)
	m2 := updated.(RootModel)
	if m2.SettingsFocusedPane != 0 {
		t.Errorf("Expected Reset key to be ignored when tabs are focused")
	}

	// Verify Tab (Browse) is ignored when tabs are focused
	tabMsg := tea.KeyPressMsg{Code: tea.KeyTab}
	updated, _ = m.Update(tabMsg)
	m3 := updated.(RootModel)
	if m3.SettingsFocusedPane != 0 {
		t.Errorf("Expected Browse key to be ignored when tabs are focused")
	}
}

func TestDisplayCategoriesExposeStoredValues(t *testing.T) {
	m := RootModel{Settings: config.DefaultSettings()}
	m.Settings.General.DefaultDownloadDir.Value = "/example/downloads"
	m.Settings.Network.GlobalRateLimit.Value = "12 MiB/s"
	for _, tc := range []struct {
		category, key string
		want          any
	}{
		{"Downloads", "default_download_dir", "/example/downloads"},
		{"Downloads", "global_rate_limit", "12 MiB/s"},
		{"Appearance", "transparent_background", false},
		{"Network", "allow_remote_open_actions", false},
	} {
		if got := m.getSettingsValues(tc.category)[tc.key]; got != tc.want {
			t.Errorf("%s/%s: got %v, want %v", tc.category, tc.key, got, tc.want)
		}
	}
}

func TestDownloadsTabShowsAndEditsConfiguredDirectory(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, nil, config.DefaultSettings(), false)
	m.width, m.height, m.state = 140, 40, SettingsState
	m.SettingsActiveTab, m.SettingsSelectedRow, m.SettingsFocusedPane = 1, 0, 1
	m.Settings.General.DefaultDownloadDir.Value = "/example/downloads"
	if !strings.Contains(m.View().Content, "/example/downloads") {
		t.Fatal("configured directory missing from view")
	}
	m = settingsPress(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.SettingsInput.Value() != "/example/downloads" {
		t.Fatalf("editing started with %q", m.SettingsInput.Value())
	}
	m.SettingsIsEditing = false
	m.Settings.General.DefaultDownloadDir.Value = ""
	if !strings.Contains(m.View().Content, "(current directory)") {
		t.Fatal("empty directory fallback is not explained")
	}
}
