package tui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
			val := m.SettingsInput.Value()

			if err := m.validateSetting(settingKey, val); err != nil {
				m.settingsError = err.Error()
				utils.Debug("Settings Validation Error: %s = %s (%v)", settingKey, val, err)
				return m, nil
			}

			_ = m.setSettingValue(currentCategory, settingKey, val)
			m.SettingsIsEditing = false
			m.settingsError = ""
			m.SettingsInput.Blur()
			return m, nil
		}

		// Pass to text input
		var cmd tea.Cmd
		m.SettingsInput, cmd = m.SettingsInput.Update(msg)
		// Clear error when typing
		if m.settingsError != "" {
			m.settingsError = ""
		}
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
				m.settingsError = ""
			}
			m.SettingsSelectedRow = 0
			return m, nil
		}
	}

	// Tab Navigation
	if key.Matches(msg, m.keys.Settings.NextTab) {
		m.SettingsActiveTab = (m.SettingsActiveTab + 1) % categoryCount
		m.SettingsSelectedRow = 0
		m.settingsError = ""
		return m, nil
	}
	if key.Matches(msg, m.keys.Settings.PrevTab) {
		m.SettingsActiveTab = (m.SettingsActiveTab - 1 + categoryCount) % categoryCount
		m.SettingsSelectedRow = 0
		m.settingsError = ""
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
			m.settingsError = ""
		}
		return m, nil
	}
	if key.Matches(msg, m.keys.Settings.Down) {
		maxRow := m.getSettingsCount() - 1
		if m.SettingsSelectedRow < maxRow {
			m.SettingsSelectedRow++
			m.settingsError = ""
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

		// Categories tab → 'Manage Categories' selected → confirm full reset
		if m.SettingsActiveTab < len(categories) && categories[m.SettingsActiveTab] == "Categories" && settingKey == "category_enabled" {
			m.state = CategoryResetConfirmState
			m.quitConfirmFocused = 0
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

func (m *RootModel) validateSetting(key, value string) error {
	trimmed := strings.TrimSpace(value)
	switch key {
	case "max_connections_per_host":
		v, err := strconv.Atoi(trimmed)
		if err != nil || v < 1 || v > 64 {
			return fmt.Errorf("must be between 1 and 64")
		}
	case "max_concurrent_downloads", "max_concurrent_probes":
		v, err := strconv.Atoi(trimmed)
		if err != nil || v < 1 || v > 10 {
			return fmt.Errorf("must be between 1 and 10")
		}
	case "min_chunk_size":
		v, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || v < 0.1 {
			return fmt.Errorf("must be at least 0.1 MB")
		}
	case "worker_buffer_size":
		v, err := strconv.Atoi(trimmed)
		if err != nil || v < 1 {
			return fmt.Errorf("must be at least 1 KB")
		}
	case "dial_hedge_count":
		v, err := strconv.Atoi(trimmed)
		if err != nil || v < 0 || v > 16 {
			return fmt.Errorf("must be between 0 and 16")
		}
	case "max_task_retries":
		v, err := strconv.Atoi(trimmed)
		if err != nil || v < 0 || v > 10 {
			return fmt.Errorf("must be between 0 and 10")
		}
	case "slow_worker_threshold", "speed_ema_alpha":
		v, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || v < 0.0 || v > 1.0 {
			return fmt.Errorf("must be between 0.0 and 1.0")
		}
	case "slow_worker_grace_period", "stall_timeout":
		if v, err := strconv.ParseFloat(trimmed, 64); err == nil {
			if v < 0 {
				return fmt.Errorf("must be non-negative")
			}
			return nil
		}
		if d, err := time.ParseDuration(trimmed); err != nil {
			return fmt.Errorf("invalid duration (e.g. 5s or 5)")
		} else if d < 0 {
			return fmt.Errorf("must be non-negative")
		}
	case "log_retention_count":
		v, err := strconv.Atoi(trimmed)
		if err != nil || v < 1 || v > 100 {
			return fmt.Errorf("must be between 1 and 100")
		}
	case "proxy_url":
		if trimmed == "" {
			return nil
		}
		u, err := url.Parse(trimmed)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("invalid URL (e.g. http://127.0.0.1:1080)")
		}
	case "custom_dns":
		return config.ValidateDNSList(trimmed)
	}
	return nil
}
