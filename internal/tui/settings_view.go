package tui

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/tui/colors"
	"github.com/surge-downloader/surge/internal/tui/components"

	"github.com/charmbracelet/lipgloss"
)

// viewSettings renders the Btop-style settings page
func (m RootModel) viewSettings() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	// Larger, more spacious modal size (responsive to terminal width)
	width := int(float64(m.width) * 0.65) // 65% of terminal width
	if width < 90 {
		width = 90
	}
	if width > 120 {
		width = 120
	}
	height := 24
	if m.width < width+4 {
		width = m.width - 4
	}
	if m.height < height+4 {
		height = m.height - 4
	}
	if width < 40 || height < 10 {
		content := lipgloss.NewStyle().
			Padding(1, 2).
			Foreground(colors.LightGray).
			Render("Terminal too small for settings view")
		box := renderBtopBox(PaneTitleStyle.Render(" Settings "), "", content, width, height, colors.NeonPurple)
		return m.renderModalWithOverlay(box)
	}

	// Get category metadata
	categories := config.CategoryOrder()
	metadata := config.GetSettingsMetadata()

	// Safety: Ensure active tab is within bounds (handling reduced category count)
	if m.SettingsActiveTab >= len(categories) {
		m.SettingsActiveTab = len(categories) - 1
	}
	if m.SettingsActiveTab < 0 {
		m.SettingsActiveTab = 0
	}

	// === TAB BAR ===
	var tabs []components.Tab
	for _, cat := range categories {
		tabs = append(tabs, components.Tab{Label: cat, Count: -1})
	}
	// Purple theme for settings tabs
	settingsActiveTab := lipgloss.NewStyle().Foreground(colors.NeonPurple)
	tabBar := components.RenderNumberedTabBar(tabs, m.SettingsActiveTab, settingsActiveTab, TabStyle)

	// === CONTENT AREA ===
	currentCategory := categories[m.SettingsActiveTab]
	settingsMeta := metadata[currentCategory]

	// Get current settings values
	settingsValues := m.getSettingsValues(currentCategory)

	// Calculate column widths - give left panel more room
	leftWidth := 32
	minRightWidth := 16
	if width-leftWidth-8 < minRightWidth {
		leftWidth = width - minRightWidth - 8
	}
	if leftWidth < 12 {
		leftWidth = 12
	}
	rightWidth := width - leftWidth - 8
	if rightWidth < minRightWidth {
		rightWidth = minRightWidth
	}

	// === LEFT COLUMN: Settings List (names only) ===
	var listLines []string
	for i, meta := range settingsMeta {
		line := meta.Label

		// Highlight selected row with better visual treatment
		if i == m.SettingsSelectedRow {
			style := lipgloss.NewStyle().Foreground(colors.NeonPurple).Bold(true)
			cursor := "▸ "

			if meta.Key == "max_global_connections" {
				style = lipgloss.NewStyle().Foreground(colors.Gray)
				cursor = "# "
			}

			line = style.Render(cursor + line)
		} else {
			style := lipgloss.NewStyle().Foreground(colors.LightGray)

			if meta.Key == "max_global_connections" {
				style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#aaaaaa", Dark: "238"}) // Darker gray
			}

			line = style.Render("  " + line)
		}

		listLines = append(listLines, line)
	}

	listContent := lipgloss.JoinVertical(lipgloss.Left, listLines...)

	// Wrap list in a bordered box with better padding
	listBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.Gray).
		Width(leftWidth).
		Padding(1, 1).
		Render(listContent)

	// === RIGHT COLUMN: Value + Description ===
	var rightContent string
	if m.SettingsSelectedRow < len(settingsMeta) {
		meta := settingsMeta[m.SettingsSelectedRow]
		value := settingsValues[meta.Key]

		// Get unit suffix
		unit := m.getSettingUnit()
		unitStyle := lipgloss.NewStyle().Foreground(colors.Gray)

		// Format value
		var valueStr string
		if m.SettingsIsEditing {
			// Show input with unit suffix (non-deletable)
			valueStr = m.SettingsInput.View() + unitStyle.Render(unit)
		} else {
			// Show formatted value with unit
			valueStr = formatSettingValueForEdit(value, meta.Type, meta.Key) + unitStyle.Render(unit)

			if meta.Key == "max_global_connections" {
				valueStr += " (Ignored)"
			}
		}

		// Show Tab hint for directory settings
		valueLabel := "Value: "
		if meta.Key == "default_download_dir" && !m.SettingsIsEditing {
			valueLabel = "[Tab] Browse: "
		}

		// Value section with better styling
		valueLabelStyle := lipgloss.NewStyle().
			Foreground(colors.NeonCyan).
			Bold(true)
		valueContentStyle := lipgloss.NewStyle().
			Foreground(colors.White)

		valueDisplay := valueLabelStyle.Render(valueLabel) + valueContentStyle.Render(valueStr)

		// Subtle divider between value and description
		dividerWidth := rightWidth - 4
		if dividerWidth < 1 {
			dividerWidth = 1
		}
		divider := lipgloss.NewStyle().
			Foreground(colors.Gray).
			Render(strings.Repeat("─", dividerWidth))

		// Description with better formatting
		descDisplay := lipgloss.NewStyle().
			Foreground(colors.LightGray).
			Width(rightWidth - 4).
			Render(meta.Description)

		rightContent = lipgloss.JoinVertical(lipgloss.Left,
			valueDisplay,
			"",
			divider,
			"",
			descDisplay,
		)
	}

	rightBox := lipgloss.NewStyle().
		Width(rightWidth).
		Padding(1, 2).
		Render(rightContent)

	// === VERTICAL DIVIDER ===
	// Calculate divider height based on listBox height
	listBoxHeight := lipgloss.Height(listBox)
	dividerStyle := lipgloss.NewStyle().
		Foreground(colors.Gray)
	if listBoxHeight < 1 {
		listBoxHeight = 1
	}
	divider := dividerStyle.Render(strings.Repeat("│\n", listBoxHeight-1) + "│")

	// === COMBINE COLUMNS ===
	content := lipgloss.JoinHorizontal(lipgloss.Top, listBox, divider, rightBox)

	// === HELP TEXT using Bubbles help ===
	helpStyle := lipgloss.NewStyle().
		Foreground(colors.Gray).
		Width(width - 6).
		Align(lipgloss.Center)
	helpText := helpStyle.Render(m.help.View(m.keys.Settings))

	// Calculate heights for proper spacing
	tabBarHeight := lipgloss.Height(tabBar)
	contentHeight := lipgloss.Height(content)
	helpHeight := lipgloss.Height(helpText)

	// innerHeight = height - 2 (top/bottom borders)
	innerHeight := height - 2
	// Used space: 1 (empty line) + tabBarHeight + 1 (empty line) + contentHeight + helpHeight
	usedHeight := 1 + tabBarHeight + 1 + contentHeight + helpHeight
	// Padding needed to push help to bottom
	paddingLines := innerHeight - usedHeight
	if paddingLines < 0 {
		paddingLines = 0
	}
	padding := strings.Repeat("\n", paddingLines)

	// === FINAL ASSEMBLY ===
	fullContent := lipgloss.JoinVertical(lipgloss.Left,
		"",
		tabBar,
		"",
		content,
		padding+helpText,
	)

	box := renderBtopBox(PaneTitleStyle.Render(" Settings "), "", fullContent, width, height, colors.NeonPurple)

	return m.renderModalWithOverlay(box)
}

// getSettingsValues returns a map of setting key -> value for a category
func (m RootModel) getSettingsValues(category string) map[string]interface{} {
	values := make(map[string]interface{})

	switch category {
	case "General":
		values["default_download_dir"] = m.Settings.General.DefaultDownloadDir
		values["warn_on_duplicate"] = m.Settings.General.WarnOnDuplicate
		values["download_complete_notification"] = m.Settings.General.DownloadCompleteNotification
		values["allow_remote_open_actions"] = m.Settings.General.AllowRemoteOpenActions
		values["extension_prompt"] = m.Settings.General.ExtensionPrompt
		values["auto_resume"] = m.Settings.General.AutoResume
		values["skip_update_check"] = m.Settings.General.SkipUpdateCheck

		values["clipboard_monitor"] = m.Settings.General.ClipboardMonitor
		values["theme"] = m.Settings.General.Theme
		values["log_retention_count"] = m.Settings.General.LogRetentionCount

	case "Network":
		values["max_connections_per_host"] = m.Settings.Network.MaxConnectionsPerHost

		values["max_concurrent_downloads"] = m.Settings.Network.MaxConcurrentDownloads
		values["user_agent"] = m.Settings.Network.UserAgent
		values["sequential_download"] = m.Settings.Network.SequentialDownload
		values["min_chunk_size"] = m.Settings.Network.MinChunkSize
		values["worker_buffer_size"] = m.Settings.Network.WorkerBufferSize
	case "Performance":
		values["max_task_retries"] = m.Settings.Performance.MaxTaskRetries
		values["slow_worker_threshold"] = m.Settings.Performance.SlowWorkerThreshold
		values["slow_worker_grace_period"] = m.Settings.Performance.SlowWorkerGracePeriod
		values["stall_timeout"] = m.Settings.Performance.StallTimeout
		values["speed_ema_alpha"] = m.Settings.Performance.SpeedEmaAlpha
	case "Categories":
		values["category_enabled"] = m.Settings.General.CategoryEnabled
	}

	return values
}

// setSettingValue sets a setting value from string input
func (m *RootModel) setSettingValue(category, key, value string) error {
	metadata := config.GetSettingsMetadata()
	metas := metadata[category]

	var meta config.SettingMeta
	for _, sm := range metas {
		if sm.Key == key {
			meta = sm
			break
		}
	}

	switch category {
	case "General":
		return m.setGeneralSetting(key, value, meta.Type)
	case "Network":
		return m.setNetworkSetting(key, value, meta.Type)
	case "Performance":
		return m.setPerformanceSetting(key, value, meta.Type)
	case "Categories":
		if key == "category_enabled" {
			m.Settings.General.CategoryEnabled = !m.Settings.General.CategoryEnabled
		}
	}

	return nil
}

func (m *RootModel) persistSettings() error {
	if err := config.SaveSettings(m.Settings); err != nil {
		return err
	}
	if reloader, ok := m.Service.(interface{ ReloadSettings() error }); ok {
		if err := reloader.ReloadSettings(); err != nil {
			return err
		}
	}
	if m.Orchestrator != nil {
		m.Orchestrator.ApplySettings(m.Settings)
	}
	return nil
}

func (m *RootModel) setGeneralSetting(key, value, typ string) error {
	switch key {
	case "default_download_dir":
		m.Settings.General.DefaultDownloadDir = value
	case "warn_on_duplicate":
		m.Settings.General.WarnOnDuplicate = !m.Settings.General.WarnOnDuplicate
	case "allow_remote_open_actions":
		m.Settings.General.AllowRemoteOpenActions = !m.Settings.General.AllowRemoteOpenActions
	case "extension_prompt":
		m.Settings.General.ExtensionPrompt = !m.Settings.General.ExtensionPrompt
	case "auto_resume":
		m.Settings.General.AutoResume = !m.Settings.General.AutoResume
	case "skip_update_check":
		m.Settings.General.SkipUpdateCheck = !m.Settings.General.SkipUpdateCheck
	case "clipboard_monitor":
		m.Settings.General.ClipboardMonitor = !m.Settings.General.ClipboardMonitor

	case "theme":
		var theme int
		valLower := strings.ToLower(value)
		switch valLower {
		case "system", "adaptive", "0":
			theme = config.ThemeAdaptive
		case "light", "1":
			theme = config.ThemeLight
		case "dark", "2":
			theme = config.ThemeDark
		default:
			// Try parsing as int fallback
			if v, err := strconv.Atoi(value); err == nil {
				if v >= 0 && v <= 2 {
					theme = v
				} else {
					return nil // Invalid range
				}
			} else {
				return nil // Invalid value
			}
		}
		m.Settings.General.Theme = theme
		m.ApplyTheme(theme)
	case "log_retention_count":
		if v, err := strconv.Atoi(value); err == nil {
			if v < 0 {
				v = 0 // Minimum valid value
			}
			m.Settings.General.LogRetentionCount = v
		}
	}
	return nil
}

func (m *RootModel) setNetworkSetting(key, value, typ string) error {
	switch key {
	case "max_connections_per_host":
		if v, err := strconv.Atoi(value); err == nil {
			m.Settings.Network.MaxConnectionsPerHost = v
		}

	case "max_concurrent_downloads":
		if v, err := strconv.Atoi(value); err == nil {
			if v < 1 {
				v = 1
			} else if v > 10 {
				v = 10
			}
			m.Settings.Network.MaxConcurrentDownloads = v
		}
	case "user_agent":
		m.Settings.Network.UserAgent = value
	case "sequential_download":
		// Toggle logic handled by generic bool toggle in Update, but just in case
		if value == "" {
			m.Settings.Network.SequentialDownload = !m.Settings.Network.SequentialDownload
		} else {
			// For programmatic setting if ever needed
			b, _ := strconv.ParseBool(value)
			m.Settings.Network.SequentialDownload = b
		}
	case "min_chunk_size":
		// Parse as MB and convert to bytes
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			m.Settings.Network.MinChunkSize = int64(v * float64(config.MB))
		}
	case "worker_buffer_size":
		// Keep buffer in KB
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			m.Settings.Network.WorkerBufferSize = int(v * float64(config.KB))
		}
	}
	return nil
}

func (m *RootModel) setPerformanceSetting(key, value, typ string) error {
	switch key {
	case "max_task_retries":
		if v, err := strconv.Atoi(value); err == nil {
			m.Settings.Performance.MaxTaskRetries = v
		}
	case "slow_worker_threshold":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			// Clamp to valid range 0.0-1.0
			if v < 0.0 {
				v = 0.0
			} else if v > 1.0 {
				v = 1.0
			}
			m.Settings.Performance.SlowWorkerThreshold = v
		}
	case "slow_worker_grace_period":
		// Check if it's just a number, if so add "s"
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			value += "s"
		}
		if v, err := time.ParseDuration(value); err == nil {
			m.Settings.Performance.SlowWorkerGracePeriod = v
		}
	case "stall_timeout":
		// Check if it's just a number, if so add "s"
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			value += "s"
		}
		if v, err := time.ParseDuration(value); err == nil {
			m.Settings.Performance.StallTimeout = v
		}
	case "speed_ema_alpha":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			// Clamp to valid range 0.0-1.0
			if v < 0.0 {
				v = 0.0
			} else if v > 1.0 {
				v = 1.0
			}
			m.Settings.Performance.SpeedEmaAlpha = v
		}
	}
	return nil
}

// getCurrentSettingKey returns the key of the currently selected setting
func (m RootModel) getCurrentSettingKey() string {
	meta := m.getCurrentSettingMeta()
	if meta != nil {
		return meta.Key
	}
	return ""
}

// getCurrentSettingMeta returns the metadata for the currently selected setting
func (m RootModel) getCurrentSettingMeta() *config.SettingMeta {
	categories := config.CategoryOrder()
	if m.SettingsActiveTab < 0 || m.SettingsActiveTab >= len(categories) {
		return nil
	}

	activeCategory := categories[m.SettingsActiveTab]
	settingsMap := config.GetSettingsMetadata()
	settingsList, ok := settingsMap[activeCategory]
	if !ok || m.SettingsSelectedRow < 0 || m.SettingsSelectedRow >= len(settingsList) {
		return nil
	}
	return &settingsList[m.SettingsSelectedRow]
}

// getCurrentSettingType returns the type of the currently selected setting
func (m RootModel) getCurrentSettingType() string {
	meta := m.getCurrentSettingMeta()
	if meta != nil {
		return meta.Type
	}
	return "string"
}

// getSettingsCount returns the number of settings in the current category
func (m RootModel) getSettingsCount() int {
	categories := config.CategoryOrder()
	if m.SettingsActiveTab >= 0 && m.SettingsActiveTab < len(categories) {
		activeCategory := categories[m.SettingsActiveTab]
		settingsMap := config.GetSettingsMetadata()

		if settingsList, ok := settingsMap[activeCategory]; ok {
			return len(settingsList)
		}
	}
	return 0
}

// getSettingUnit returns the unit suffix for the currently selected setting
func (m RootModel) getSettingUnit() string {
	key := m.getCurrentSettingKey()
	switch key {
	case "min_chunk_size":
		return " MB"
	case "worker_buffer_size":
		return " KB"
	case "max_task_retries":
		return " retries"
	case "slow_worker_grace_period", "stall_timeout":
		return " seconds"
	case "slow_worker_threshold", "speed_ema_alpha":
		return " (0.0-1.0)"
	default:
		return ""
	}
}

// formatSettingValueForEdit returns a plain value without units for editing
func formatSettingValueForEdit(value interface{}, typ, key string) string {
	switch key {
	case "min_chunk_size":
		if v, ok := value.(int64); ok {
			mb := float64(v) / float64(config.MB)
			return fmt.Sprintf("%.1f", mb)
		}
	case "worker_buffer_size":
		v := reflect.ValueOf(value)
		if v.Kind() == reflect.Int {
			kb := float64(v.Int()) / float64(config.KB)
			return fmt.Sprintf("%.0f", kb)
		}
	case "slow_worker_grace_period", "stall_timeout":
		// Show duration as plain seconds number (e.g., "5" instead of "5s")
		if d, ok := value.(time.Duration); ok {
			return fmt.Sprintf("%.0f", d.Seconds())
		}
	}

	if key == "theme" {
		if v, ok := value.(int); ok {
			switch v {
			case config.ThemeAdaptive:
				return "< System >"
			case config.ThemeLight:
				return "< Light >"
			case config.ThemeDark:
				return "< Dark >"
			}
		}
	}

	// Default: use standard format
	return formatSettingValue(value, typ)
}

// formatSettingValue formats a setting value for display
func formatSettingValue(value interface{}, typ string) string {
	if value == nil {
		return "-"
	}

	switch typ {
	case "bool":
		if b, ok := value.(bool); ok {
			if b {
				return "True"
			}
			return "False"
		}
	case "duration":
		if d, ok := value.(time.Duration); ok {
			return d.String()
		}
	case "int64":
		if v, ok := value.(int64); ok {
			// Just display the raw number - units handled by getSettingUnit
			return fmt.Sprintf("%d", v)
		}
	case "int":
		v := reflect.ValueOf(value)
		if v.Kind() == reflect.Int {
			return fmt.Sprintf("%d", v.Int())
		}
	case "float64":
		if v, ok := value.(float64); ok {
			return fmt.Sprintf("%.2f", v)
		}
	case "string":
		if s, ok := value.(string); ok {
			if s == "" {
				return "(default)"
			}
			if len(s) > 30 {
				return s[:27] + "..."
			}
			return s
		}
	}

	// Fallback using reflection for numeric types
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Int, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Float64:
		return fmt.Sprintf("%.2f", v.Float())
	default:
		return fmt.Sprintf("%v", value)
	}
}

// resetSettingToDefault resets a specific setting to its default value
func (m *RootModel) resetSettingToDefault(category, key string, defaults *config.Settings) {
	switch category {
	case "General":
		switch key {
		case "default_download_dir":
			m.Settings.General.DefaultDownloadDir = defaults.General.DefaultDownloadDir
		case "warn_on_duplicate":
			m.Settings.General.WarnOnDuplicate = defaults.General.WarnOnDuplicate
		case "download_complete_notification":
			m.Settings.General.DownloadCompleteNotification = defaults.General.DownloadCompleteNotification
		case "extension_prompt":
			m.Settings.General.ExtensionPrompt = defaults.General.ExtensionPrompt
		case "auto_resume":
			m.Settings.General.AutoResume = defaults.General.AutoResume
		case "skip_update_check":
			m.Settings.General.SkipUpdateCheck = defaults.General.SkipUpdateCheck

		case "clipboard_monitor":
			m.Settings.General.ClipboardMonitor = defaults.General.ClipboardMonitor
		case "theme":
			m.Settings.General.Theme = defaults.General.Theme
		case "log_retention_count":
			m.Settings.General.LogRetentionCount = defaults.General.LogRetentionCount
		}

	case "Network":
		// Handle Network-related keys
		switch key {
		case "max_connections_per_host":
			m.Settings.Network.MaxConnectionsPerHost = defaults.Network.MaxConnectionsPerHost

		case "max_concurrent_downloads":
			m.Settings.Network.MaxConcurrentDownloads = defaults.Network.MaxConcurrentDownloads
		case "user_agent":
			m.Settings.Network.UserAgent = defaults.Network.UserAgent
		case "sequential_download":
			m.Settings.Network.SequentialDownload = defaults.Network.SequentialDownload
		case "min_chunk_size":
			m.Settings.Network.MinChunkSize = defaults.Network.MinChunkSize
		case "worker_buffer_size":
			m.Settings.Network.WorkerBufferSize = defaults.Network.WorkerBufferSize
		}
	case "Performance":
		switch key {
		case "max_task_retries":
			m.Settings.Performance.MaxTaskRetries = defaults.Performance.MaxTaskRetries
		case "slow_worker_threshold":
			m.Settings.Performance.SlowWorkerThreshold = defaults.Performance.SlowWorkerThreshold
		case "slow_worker_grace_period":
			m.Settings.Performance.SlowWorkerGracePeriod = defaults.Performance.SlowWorkerGracePeriod
		case "stall_timeout":
			m.Settings.Performance.StallTimeout = defaults.Performance.StallTimeout
		case "speed_ema_alpha":
			m.Settings.Performance.SpeedEmaAlpha = defaults.Performance.SpeedEmaAlpha
		}
	case "Categories":
		switch key {
		case "category_enabled":
			m.Settings.General.CategoryEnabled = defaults.General.CategoryEnabled
		}
	}
}
