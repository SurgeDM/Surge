package tui

import (
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/clipboard"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/utils"
)

type extensionTokenFlashFadeMsg struct{}

func (m RootModel) updateSettings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.SettingsBaseline == nil {
		m.snapshotSettings()
	}
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
		requiresRestart := m.checkRestartRequirement()
		// Save settings and exit
		_ = m.persistSettings()
		if requiresRestart {
			m.state = RestartConfirmState
			m.quitConfirmFocused = 0
			return m, nil
		}
		m.state = DashboardState
		m.SettingsBaseline = nil
		return m, nil
	}
	tabBindings := []key.Binding{
		m.keys.Settings.Tab1,
		m.keys.Settings.Tab2,
		m.keys.Settings.Tab3,
		m.keys.Settings.Tab4,
		m.keys.Settings.Tab5,
	}
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

	// Open file browser for default_download_dir or theme_path
	if key.Matches(msg, m.keys.Settings.Browse) {
		settingKey := m.getCurrentSettingKey()
		switch settingKey {
		case "default_download_dir":
			originalPath := m.Settings.General.DefaultDownloadDir
			browseDir := originalPath
			if browseDir == "" {
				browseDir = m.PWD
			}
			return m, m.openDirectoryPicker(FilePickerOriginSettings, originalPath, browseDir, false, true)
		case "theme_path":
			originalPath := m.Settings.General.ThemePath
			browseDir := originalPath
			if browseDir != "" {
				if info, err := os.Stat(browseDir); err == nil && !info.IsDir() {
					browseDir = filepath.Dir(browseDir)
				}
			}
			if browseDir == "" {
				browseDir = config.GetThemesDir()
			}
			if browseDir == "" {
				browseDir = m.PWD
			}
			cmd := m.openDirectoryPicker(FilePickerOriginTheme, originalPath, browseDir, true, false)
			m.filepicker.AllowedTypes = []string{".toml"}
			return m, cmd
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
			m.ApplyTheme(newTheme, m.Settings.General.ThemePath)
			return m, nil
		}

		// Toggle bool or enter edit mode for other types
		typ := m.getCurrentSettingType()

		// Special actions for custom types
		if typ == "auth_token" {
			token := GetAuthToken()
			if token != "" {
				_ = clipboard.Write(token)
				m.ExtensionTokenCopied = true
				return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
					return extensionTokenFlashFadeMsg{}
				})
			}
			return m, nil
		}

		if typ == "link" {
			currentCategory := categories[m.SettingsActiveTab]
			values := m.getSettingsValues(currentCategory)
			if url, ok := values[settingKey].(string); ok && url != "" {
				_ = utils.OpenBrowser(url)
			}
			return m, nil
		}

		currentCategory := categories[m.SettingsActiveTab]
		if typ == "bool" {
			_ = m.setSettingValue(currentCategory, settingKey, "")
		} else {
			// Enter edit mode
			m.SettingsIsEditing = true
			// Pre-fill with current value (without units)
			values := m.getSettingsValues(currentCategory)
			m.SettingsInput.SetValue(formatSettingValueForEdit(values[settingKey], typ, settingKey, false))
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
		if settingKey == "theme" || settingKey == "theme_path" {
			m.ApplyTheme(m.Settings.General.Theme, m.Settings.General.ThemePath)
		}
		return m, nil
	}

	return m, nil
}
