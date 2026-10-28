package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/SurgeDM/Surge/internal/tui/components"
)

type settingsActionKind uint8

const (
	settingsResetOne settingsActionKind = iota
	settingsResetAll
	settingsDeleteCategory
)

type settingsAction struct {
	kind                 settingsActionKind
	category, key, label string
	index                int
	rule                 config.Category
}

func (m RootModel) categoryListFocused() bool {
	typ := m.getCurrentSettingType()
	return m.SettingsFocusedPane == 1 && (typ == config.TypeCustomCategory || typ == config.TypeCustomCategoryAdd)
}

func (m RootModel) requestSettingsConfirmation(kind settingsActionKind) RootModel {
	if m.SettingsReadOnly {
		m.settingsError = "This daemon does not support editing settings"
		return m
	}
	action := &settingsAction{kind: kind}
	switch kind {
	case settingsResetOne:
		action.category, action.key = m.currentSettingsCategory(), m.getCurrentSettingKey()
		if action.key == "max_global_connections" {
			return m
		}
		setting := m.Settings.FindSetting(action.category, action.key)
		if setting == nil {
			return m
		} // Category rules and Add are not settings to reset.
		action.label = setting.Label
	case settingsDeleteCategory:
		if m.getCurrentSettingType() != config.TypeCustomCategory {
			return m
		}
		index, err := strconv.Atoi(strings.TrimPrefix(m.getCurrentSettingKey(), "category_"))
		if err != nil || index < 0 || index >= len(m.Settings.Categories.Categories) {
			return m
		}
		action.index = index
		action.rule = m.Settings.Categories.Categories[index]
		action.label = action.rule.Name
	}
	m.settingsAction = action
	m.settingsError = ""
	m.quitConfirmFocused = 1 // Default to Cancel for every reset or deletion.
	m.state = SettingsConfirmState
	return m
}

func (m RootModel) updateSettingsConfirmation(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m, decision, handled := m.handleYesNoSelection(msg)
	if !handled || decision == yesNoNone {
		return m, nil
	}
	action := m.settingsAction
	m.settingsAction = nil
	m.state = SettingsState
	if decision != yesNoYes || action == nil {
		return m, nil
	}
	if m.SettingsReadOnly {
		m.settingsError = "This daemon does not support editing settings"
		return m, nil
	}
	defaults := config.DefaultSettings()
	switch action.kind {
	case settingsDeleteCategory:
		cats := m.Settings.Categories.Categories
		if action.index < 0 || action.index >= len(cats) || cats[action.index] != action.rule {
			m.settingsError = "Category changed; select it again before deleting"
			return m, nil
		}
		m.Settings.Categories.Categories = append(cats[:action.index], cats[action.index+1:]...)
	case settingsResetOne:
		if err := m.resetSettingToDefault(action.category, action.key, defaults); err != nil {
			m.settingsError = err.Error()
			return m, nil
		}
		if action.key == "theme" || action.key == "theme_path" {
			m.ApplyTheme(config.Resolve[int](m.Settings.General.Theme), config.Resolve[string](m.Settings.General.ThemePath))
		}
	case settingsResetAll:
		// Apply the existing service transition before replacing the settings.
		// If it fails, preserve the rest of the user's settings.
		if err := m.resetSettingToDefault("General", "auto_start", defaults); err != nil {
			m.settingsError = err.Error()
			return m, nil
		}
		if m.IsRemote {
			defaults.General.DefaultDownloadDir.Value = m.Settings.General.DefaultDownloadDir.Value
		}
		m.Settings = defaults
		m.ApplyTheme(config.Resolve[int](defaults.General.Theme), config.Resolve[string](defaults.General.ThemePath))
		m.applyAutoShutdownSettingChange()
	}
	m.settingsError = ""
	m.normalizeSettingsSelection()
	return m, nil
}

func (m RootModel) viewSettingsConfirmation() string {
	title, message, detail, confirm := "Reset setting", "Reset the selected setting?", "Only this setting will be reset.", "Reset"
	border := colors.Orange()
	if action := m.settingsAction; action != nil {
		switch action.kind {
		case settingsResetOne:
			message = fmt.Sprintf("Reset %q to its default value?", action.label)
		case settingsResetAll:
			title, message, confirm = "Reset all settings", "Reset all settings to their defaults?", "Reset all"
			detail = "Category rules will also reset. Custom keybindings are kept."
		case settingsDeleteCategory:
			title, message, confirm = "Delete category", fmt.Sprintf("Delete category %q?", action.label), "Delete"
			detail = "Existing downloads and files will be kept."
			border = colors.Red()
		}
	}
	w, h := GetDynamicModalDimensions(m.width, m.height, 40, 8, 76, 12)
	modal := components.ConfirmationModal{
		Title: title, Message: message, Detail: detail,
		Keys: m.keys.QuitConfirm, Help: m.help,
		BorderColor: border, ButtonColor: border, Width: w, Height: h,
		ShowYesNoButtons: true, YesNoFocused: m.quitConfirmFocused,
		YesLabel: confirm, NoLabel: "Cancel",
	}
	return modal.RenderWithBtopBox(renderBtopBox, PaneTitleStyle)
}
