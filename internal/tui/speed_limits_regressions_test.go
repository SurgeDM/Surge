package tui

import (
	"testing"

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
