package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func TestSettingsNavigation_VimStylePaneTransitions(t *testing.T) {
	// 1. Initialize RootModel with default keymap and settings
	keys := DefaultKeyMap()
	settings := config.DefaultSettings()

	m := RootModel{
		state:               SettingsState,
		keys:                keys,
		Settings:            settings,
		SettingsActiveTab:   0,
		SettingsSelectedRow: 0,
		SettingsFocusedPane: 1, // Start with List focused
	}

	// 2. Press "k" (Up) at row 0 -> should transition focus up to Tab bar
	upMsg := tea.KeyPressMsg{Code: 'k', Text: "k"}
	updated, _ := m.Update(upMsg)
	m = updated.(RootModel)

	if m.SettingsFocusedPane != 0 {
		t.Errorf("Expected focus to transition to Tab bar (0) when pressing Up on first row, got %d", m.SettingsFocusedPane)
	}

	// 3. Press "l" (NextTab/right) while focused on Tab bar -> should shift active tab to 1
	rightMsg := tea.KeyPressMsg{Code: 'l', Text: "l"}
	updated, _ = m.Update(rightMsg)
	m = updated.(RootModel)

	if m.SettingsActiveTab != 1 {
		t.Errorf("Expected active tab to shift to 1 when pressing NextTab on tab bar, got %d", m.SettingsActiveTab)
	}
	if m.SettingsFocusedPane != 0 {
		t.Errorf("Expected focus to remain on Tab bar after shifting tab, got %d", m.SettingsFocusedPane)
	}

	// 4. Press "j" (Down) while focused on Tab bar -> should transition focus down to Settings List (row 0)
	downMsg := tea.KeyPressMsg{Code: 'j', Text: "j"}
	updated, _ = m.Update(downMsg)
	m = updated.(RootModel)

	if m.SettingsFocusedPane != 1 {
		t.Errorf("Expected focus to transition to List (1) when pressing Down on tab bar, got %d", m.SettingsFocusedPane)
	}
	if m.SettingsSelectedRow != 0 {
		t.Errorf("Expected selected row to be reset to 0 upon entering list, got %d", m.SettingsSelectedRow)
	}

	// 5. Press "j" (Down) while focused on List -> should move selection to row 1
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

	// 7. Go up to tabs again
	m.SettingsSelectedRow = 0
	updated, _ = m.Update(upMsg)
	m = updated.(RootModel)
	if m.SettingsFocusedPane != 0 {
		t.Fatalf("Failed to refocus tab bar")
	}

	// 8. Press "enter" (Edit/confirm) while on tabs -> should shift focus to list
	enterMsg := tea.KeyPressMsg{Code: tea.KeyEnter}
	updated, _ = m.Update(enterMsg)
	m = updated.(RootModel)
	if m.SettingsFocusedPane != 1 {
		t.Errorf("Expected enter key on tabs to shift focus to settings list, got %d", m.SettingsFocusedPane)
	}
}

func TestSettingsNavigation_ResetAndBrowseGuards(t *testing.T) {
	keys := DefaultKeyMap()
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
