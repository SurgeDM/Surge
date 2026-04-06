package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func (m RootModel) updateSettings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.normalizeSettingsSelection()

	categories := config.CategoryOrder()
	categoryCount := len(categories)
	if categoryCount == 0 {
		return m, nil
	}

	// Handle editing mode first
	if m.SettingsIsEditing {
		if key.Matches(msg, m.keys.SettingsEditor.Cancel) {
			// Cancel editing
			m.SettingsIsEditing = false
			m.SettingsInput.Blur()
			return m, nil
		}
		if key.Matches(msg, m.keys.SettingsEditor.Confirm) {
			currentCategory := categories[m.SettingsActiveTab]
			settingKey := m.getCurrentSettingKey()
			_ = m.setSettingValue(currentCategory, settingKey, m.SettingsInput.Value())
			m.SettingsIsEditing = false
			m.SettingsInput.Blur()
			return m, nil
		}

		// Pass to text input
		var cmd tea.Cmd
		m.SettingsInput, cmd = m.SettingsInput.Update(msg)
		return m, cmd
	}

	// Not editing - handle navigation
	if key.Matches(msg, m.keys.Settings.Close) {
		// Save settings and exit
		_ = m.persistSettings()
		m.state = DashboardState
		return m, nil
	}
	tabBindings := []key.Binding{m.keys.Settings.Tab1, m.keys.Settings.Tab2, m.keys.Settings.Tab3, m.keys.Settings.Tab4}
	for i, binding := range tabBindings {
		if key.Matches(msg, binding) {
			if categoryCount > i {
				m.SettingsActiveTab = i
			}
			m.SettingsSelectedRow = 0
			return m, nil
		}
	}

	// Tab Navigation
	if key.Matches(msg, m.keys.Settings.NextTab) {
		m.SettingsActiveTab = (m.SettingsActiveTab + 1) % categoryCount
		m.SettingsSelectedRow = 0
		return m, nil
	}
	if key.Matches(msg, m.keys.Settings.PrevTab) {
		m.SettingsActiveTab = (m.SettingsActiveTab - 1 + categoryCount) % categoryCount
		m.SettingsSelectedRow = 0
		return m, nil
	}

	// Open file browser for default_download_dir
	if key.Matches(msg, m.keys.Settings.Browse) {
		settingKey := m.getCurrentSettingKey()
		if settingKey == "default_download_dir" {
			m.SettingsFileBrowsing = true
			m.state = FilePickerState
			m.filepicker = newFilepicker(m.Settings.General.DefaultDownloadDir)
			return m, m.filepicker.Init()
		}
		return m, nil
	}

	// Back tab - not currently bound, ignoring or could use Shift+Tab manual check if really needed
	// For now, we rely on Tab (Browse) to cycle.

	// Up/Down navigation
	if key.Matches(msg, m.keys.Settings.Up) {
		if m.SettingsSelectedRow > 0 {
			m.SettingsSelectedRow--
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Settings.Down) {
		maxRow := m.getSettingsCount() - 1
		if m.SettingsSelectedRow < maxRow {
			m.SettingsSelectedRow++
		}
		return m, nil
	}

	// Edit / Toggle
	if key.Matches(msg, m.keys.Settings.Edit) {
		// Categories tab → open Category Manager
		if m.SettingsActiveTab < len(categories) && categories[m.SettingsActiveTab] == "Categories" {
			m.catMgrCursor = 0
			m.state = CategoryManagerState
			return m, nil
		}

		settingKey := m.getCurrentSettingKey()
		// Prevent editing ignored settings
		if settingKey == "max_global_connections" {
			return m, nil
		}

		// Special handling for Theme cycling
		if settingKey == "theme" {
			newTheme := (m.Settings.General.Theme + 1) % 3
			m.Settings.General.Theme = newTheme
			m.ApplyTheme(newTheme)
			return m, nil
		}

		// Toggle bool or enter edit mode for other types
		typ := m.getCurrentSettingType()

		currentCategory := categories[m.SettingsActiveTab]
		if typ == "bool" {
			_ = m.setSettingValue(currentCategory, settingKey, "")
		} else {
			// Enter edit mode
			m.SettingsIsEditing = true
			// Pre-fill with current value (without units)
			values := m.getSettingsValues(currentCategory)
			m.SettingsInput.SetValue(formatSettingValueForEdit(values[settingKey], typ, settingKey))
			m.updateSettingsInputWidthForViewport()
			m.SettingsInput.Focus()
		}
		return m, nil
	}

	// Reset
	if key.Matches(msg, m.keys.Settings.Reset) {
		settingKey := m.getCurrentSettingKey()
		if settingKey == "max_global_connections" {
			return m, nil
		}
		defaults := config.DefaultSettings()
		currentCategory := categories[m.SettingsActiveTab]
		m.resetSettingToDefault(currentCategory, settingKey, defaults)
		if settingKey == "theme" {
			m.ApplyTheme(m.Settings.General.Theme)
		}
		return m, nil
	}

	return m, nil
}
