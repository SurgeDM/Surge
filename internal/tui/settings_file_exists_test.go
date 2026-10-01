package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func TestSettings_FileExistsActionCycles(t *testing.T) {
	m := RootModel{
		state:               SettingsState,
		keys:                config.DefaultKeyMap(),
		Settings:            config.DefaultSettings(),
		SettingsActiveTab:   0,
		SettingsFocusedPane: 1,
	}

	row := -1
	for i, meta := range m.buildSettingsMetaForCategory("General") {
		if meta.Key == "file_exists_action" {
			row = i
			break
		}
	}
	if row < 0 {
		t.Fatal("file_exists_action not listed in General settings")
	}
	m.SettingsSelectedRow = row

	if got := formatSettingValueForEdit(config.Resolve[string](m.Settings.General.FileExistsAction), config.TypeString, config.UnitNone, "file_exists_action", true); got != "< Rename >" {
		t.Fatalf("display = %q, want %q", got, "< Rename >")
	}

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	want := []string{config.FileExistsOverwrite, config.FileExistsRename}
	for _, w := range want {
		updated, _ := m.Update(enter)
		m = updated.(RootModel)
		if got := config.Resolve[string](m.Settings.General.FileExistsAction); got != w {
			t.Fatalf("after Enter: file_exists_action = %q, want %q", got, w)
		}
		if m.SettingsIsEditing {
			t.Fatal("cycling should not enter text edit mode")
		}
	}
}
