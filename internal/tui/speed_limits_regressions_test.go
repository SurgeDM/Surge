package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func newSpeedLimitsTestModel(t *testing.T) RootModel {
	t.Helper()
	settings := config.DefaultSettings()
	return RootModel{
		Settings:      settings,
		keys:          config.DefaultKeyMap(),
		SettingsInput: textinput.New(),
		state:         SpeedLimitsState,
	}
}

func TestSpeedLimitsEditingUsesSettingsEditorKeys(t *testing.T) {
	m := newSpeedLimitsTestModel(t)
	m.width, m.height = 100, 30
	m.speedLimitsIsEditing = true
	m.SettingsInput.SetValue("1")
	m.keys.SettingsEditor.Confirm = key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save"))
	m.keys.SettingsEditor.Cancel = key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl+x", "cancel"))

	if help := stripANSI.ReplaceAllString(m.viewSpeedLimits(), ""); !strings.Contains(help, "ctrl+s") || !strings.Contains(help, "ctrl+x") {
		t.Fatalf("editing help does not show editor keys: %q", help)
	}

	updated, _ := m.updateSpeedLimits(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !updated.(RootModel).speedLimitsIsEditing {
		t.Fatal("default Enter unexpectedly confirmed a custom-key edit")
	}

	updated, _ = m.updateSpeedLimits(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if updated.(RootModel).speedLimitsIsEditing {
		t.Fatal("configured editor confirm key did not finish editing")
	}
}

func TestUpdateSpeedLimits_InvalidInputSetsErrorAndRetainsFocus(t *testing.T) {
	m := newSpeedLimitsTestModel(t)
	m.speedLimitsIsEditing = true
	m.speedLimitsCursor = 0 // Global Rate Limit
	m.SettingsInput.SetValue("invalid_limit")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := updated.(RootModel)

	if m2.speedLimitsError == "" {
		t.Fatal("expected error to be set, got empty string")
	}
	if !m2.speedLimitsIsEditing {
		t.Fatal("expected to still be editing after an error")
	}
}

func TestUpdateSpeedLimits_EscClearsError(t *testing.T) {
	m := newSpeedLimitsTestModel(t)
	m.speedLimitsIsEditing = true
	m.speedLimitsError = "some error"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m2 := updated.(RootModel)

	if m2.speedLimitsError != "" {
		t.Fatalf("expected error to be cleared on Esc, got: %q", m2.speedLimitsError)
	}
	if m2.speedLimitsIsEditing {
		t.Fatal("expected to exit editing mode on Esc")
	}
}

func TestUpdateSpeedLimits_ArrowNavClearsError(t *testing.T) {
	m := newSpeedLimitsTestModel(t)
	m.speedLimitsError = "some error"

	// Test Up arrow
	updatedUp, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	mUp := updatedUp.(RootModel)
	if mUp.speedLimitsError != "" {
		t.Fatal("expected error to be cleared on Up arrow")
	}

	m.speedLimitsError = "some error"
	// Test Down arrow
	updatedDown, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	mDown := updatedDown.(RootModel)
	if mDown.speedLimitsError != "" {
		t.Fatal("expected error to be cleared on Down arrow")
	}
}

func TestSpeedLimitsPreserveBinaryMegabytes(t *testing.T) {
	m := newSpeedLimitsTestModel(t)
	m.downloads = []*DownloadModel{{ID: "download"}}
	for _, key := range []string{"global_rate_limit", "default_download_rate_limit", "dl:download"} {
		t.Run(key, func(t *testing.T) {
			value := "10 MiB/s"
			for i := 0; i < 3; i++ {
				if err := m.setSpeedLimitValue(key, value); err != nil {
					t.Fatal(err)
				}
				value = m.getSpeedLimitsValues()[key].(string)
				if value != "10 MiB/s" {
					t.Fatalf("saved limit = %q, want 10 MiB/s", value)
				}
			}
		})
	}
}

func TestSpeedLimitsSaveAndReopen(t *testing.T) {
	for cursor, name := range []string{"global", "default", "download"} {
		for _, input := range []string{"10", "100", "0.00000095367431640625", "1.234375"} {
			t.Run(name+"/"+input, func(t *testing.T) {
				m := newSpeedLimitsTestModel(t)
				m.downloads = []*DownloadModel{{ID: "download"}}
				m.speedLimitsCursor = cursor
				m.speedLimitsIsEditing = true
				m.SettingsInput.SetValue(input)
				for i := 0; i < 3; i++ {
					updated, _ := m.updateSpeedLimits(tea.KeyPressMsg{Code: tea.KeyEnter})
					m = updated.(RootModel)
					if m.speedLimitsIsEditing || m.speedLimitsError != "" {
						t.Fatalf("save failed: %s", m.speedLimitsError)
					}
					updated, _ = m.updateSpeedLimits(tea.KeyPressMsg{Code: tea.KeyEnter})
					m = updated.(RootModel)
					if got := m.SettingsInput.Value(); got != input {
						t.Fatalf("reopened input = %q, want %q", got, input)
					}
				}
			})
		}
	}
}
