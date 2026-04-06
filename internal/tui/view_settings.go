package tui

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/SurgeDM/Surge/internal/tui/components"

	"charm.land/lipgloss/v2"
)

// viewSettings renders the Btop-style settings page
func (m RootModel) viewSettings() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	width, height := settingsModalDimensions(m.width, m.height)
	if width < 40 || height < 10 {
		content := lipgloss.NewStyle().
			Padding(1, 2).
			Foreground(colors.LightGray).
			Render("Terminal too small for settings view")
		box := renderBtopBox(PaneTitleStyle.Render(" Settings "), "", content, width, height, colors.NeonPurple)
		return m.renderModalWithOverlay(box)
	}

	categories := config.CategoryOrder()
	if len(categories) == 0 {
		content := lipgloss.NewStyle().
			Padding(1, 2).
			Foreground(colors.LightGray).
			Render("No settings categories available")
		box := renderBtopBox(PaneTitleStyle.Render(" Settings "), "", content, width, height, colors.NeonPurple)
		return m.renderModalWithOverlay(box)
	}

	metadata := config.GetSettingsMetadata()
	activeTab := m.SettingsActiveTab
	if activeTab < 0 {
		activeTab = 0
	}
	if activeTab >= len(categories) {
		activeTab = len(categories) - 1
	}

	currentCategory := categories[activeTab]
	settingsMeta := metadata[currentCategory]
	if len(settingsMeta) == 0 {
		content := lipgloss.NewStyle().
			Padding(1, 2).
			Foreground(colors.LightGray).
			Render("No settings available in this category")
		box := renderBtopBox(PaneTitleStyle.Render(" Settings "), "", content, width, height, colors.NeonPurple)
		return m.renderModalWithOverlay(box)
	}

	selectedRow := m.SettingsSelectedRow
	if selectedRow < 0 {
		selectedRow = 0
	}
	if selectedRow >= len(settingsMeta) {
		selectedRow = len(settingsMeta) - 1
	}

	settingsValues := m.getSettingsValues(currentCategory)
	tabBar := m.renderSettingsTabBar(categories, activeTab, width-6)
	helpText := m.renderSettingsHelp(width - 6)

	innerHeight := height - 2
	tabBarHeight := lipgloss.Height(tabBar)
	helpHeight := lipgloss.Height(helpText)
	bodyHeight := innerHeight - tabBarHeight - helpHeight - 2 // one line gap above body and help
	if bodyHeight < 3 {
		bodyHeight = 3
	}

	var content string
	if width >= 72 && bodyHeight >= 8 {
		content = m.renderSettingsTwoColumn(settingsMeta, selectedRow, settingsValues, width, bodyHeight)
	} else {
		content = m.renderSettingsCompact(settingsMeta, selectedRow, settingsValues, width, bodyHeight)
	}

	contentHeight := lipgloss.Height(content)
	usedHeight := tabBarHeight + 1 + contentHeight + 1 + helpHeight
	paddingLines := innerHeight - usedHeight
	if paddingLines < 0 {
		paddingLines = 0
	}
	padding := strings.Repeat("\n", paddingLines)

	fullContent := lipgloss.JoinVertical(lipgloss.Left,
		tabBar,
		"",
		content,
		padding,
		helpText,
	)

	box := renderBtopBox(PaneTitleStyle.Render(" Settings "), "", fullContent, width, height, colors.NeonPurple)
	return m.renderModalWithOverlay(box)
}

func settingsModalDimensions(termWidth, termHeight int) (int, int) {
	width := int(float64(termWidth) * 0.68)
	if width < 64 {
		width = 64
	}
	if width > 120 {
		width = 120
	}
	height := 24

	maxWidth := termWidth - 4
	if maxWidth < 1 {
		maxWidth = 1
	}
	maxHeight := termHeight - 4
	if maxHeight < 1 {
		maxHeight = 1
	}

	if width > maxWidth {
		width = maxWidth
	}
	if height > maxHeight {
		height = maxHeight
	}

	return width, height
}

func shortSettingsCategoryLabel(label string) string {
	switch label {
	case "General":
		return "Gen"
	case "Network":
		return "Net"
	case "Performance":
		return "Perf"
	case "Categories":
		return "Cats"
	default:
		return label
	}
}

func (m RootModel) renderSettingsTabBar(categories []string, activeTab int, maxWidth int) string {
	if maxWidth < 1 {
		maxWidth = 1
	}

	makeTabs := func(useShort bool) []components.Tab {
		tabs := make([]components.Tab, 0, len(categories))
		for _, cat := range categories {
			label := cat
			if useShort {
				label = shortSettingsCategoryLabel(cat)
			}
			tabs = append(tabs, components.Tab{Label: label, Count: -1})
		}
		return tabs
	}

	settingsActiveTab := lipgloss.NewStyle().Foreground(colors.NeonPurple)
	tryBars := []string{
		components.RenderNumberedTabBar(makeTabs(false), activeTab, settingsActiveTab, TabStyle),
		components.RenderTabBar(makeTabs(false), activeTab, settingsActiveTab, TabStyle),
		components.RenderTabBar(makeTabs(true), activeTab, settingsActiveTab, TabStyle),
	}

	for _, candidate := range tryBars {
		if lipgloss.Width(candidate) <= maxWidth {
			return lipgloss.NewStyle().Width(maxWidth).Align(lipgloss.Center).Render(candidate)
		}
	}

	fallback := fmt.Sprintf("[%d/%d] %s", activeTab+1, len(categories), categories[activeTab])
	return lipgloss.NewStyle().
		Foreground(colors.Gray).
		Width(maxWidth).
		Align(lipgloss.Center).
		Render(fallback)
}

func (m RootModel) renderSettingsHelp(width int) string {
	if width < 1 {
		width = 1
	}

	helpText := m.help.View(m.keys.Settings)
	if width < 60 {
		helpText = "esc: save/close  tab: next tab  enter: edit"
	}
	if width < 40 {
		helpText = "esc close | enter edit"
	}

	return lipgloss.NewStyle().
		Foreground(colors.Gray).
		Width(width).
		Align(lipgloss.Center).
		Render(helpText)
}

func formatSettingsBlock(content string, width, rows int) string {
	if width < 1 {
		width = 1
	}
	if rows < 1 {
		rows = 1
	}

	lines := strings.Split(content, "\n")
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}

	for i := range lines {
		lines[i] = lipgloss.NewStyle().Width(width).MaxWidth(width).Render(lines[i])
	}

	return strings.Join(lines, "\n")
}

func renderSettingsListViewport(settingsMeta []config.SettingMeta, selectedRow, rows, innerWidth int) string {
	if rows < 1 {
		rows = 1
	}
	if innerWidth < 1 {
		innerWidth = 1
	}

	if len(settingsMeta) == 0 {
		return formatSettingsBlock("(No settings)", innerWidth, rows)
	}

	if selectedRow < 0 {
		selectedRow = 0
	}
	if selectedRow >= len(settingsMeta) {
		selectedRow = len(settingsMeta) - 1
	}

	start := 0
	if selectedRow >= rows {
		start = selectedRow - rows + 1
	}
	maxStart := len(settingsMeta) - rows
	if maxStart < 0 {
		maxStart = 0
	}
	if start > maxStart {
		start = maxStart
	}

	lines := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		idx := start + i
		if idx >= len(settingsMeta) {
			lines = append(lines, "")
			continue
		}

		meta := settingsMeta[idx]
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(colors.LightGray)
		if idx == selectedRow {
			prefix = "▸ "
			style = lipgloss.NewStyle().Foreground(colors.NeonPurple).Bold(true)
		}

		if meta.Key == "max_global_connections" {
			style = lipgloss.NewStyle().Foreground(colors.ThemeColor("#aaaaaa", "238"))
			if idx == selectedRow {
				prefix = "# "
				style = lipgloss.NewStyle().Foreground(colors.Gray)
			}
		}

		lines = append(lines, style.Width(innerWidth).MaxWidth(innerWidth).Render(prefix+meta.Label))
	}

	return strings.Join(lines, "\n")
}

func (m RootModel) renderSettingsDetailBlock(settingsMeta []config.SettingMeta, selectedRow int, settingsValues map[string]interface{}, innerWidth, rows int) string {
	if innerWidth < 1 {
		innerWidth = 1
	}
	if rows < 1 {
		rows = 1
	}
	if len(settingsMeta) == 0 || selectedRow < 0 || selectedRow >= len(settingsMeta) {
		return formatSettingsBlock("No setting selected", innerWidth, rows)
	}

	meta := settingsMeta[selectedRow]
	value := settingsValues[meta.Key]
	unit := m.getSettingUnit()
	unitStyle := lipgloss.NewStyle().Foreground(colors.Gray)

	var valueStr string
	if m.SettingsIsEditing {
		valueStr = m.SettingsInput.View() + unitStyle.Render(unit)
	} else {
		valueStr = formatSettingValueForEdit(value, meta.Type, meta.Key) + unitStyle.Render(unit)
		if meta.Key == "max_global_connections" {
			valueStr += " (Ignored)"
		}
	}

	valueLabel := "Value: "
	if meta.Key == "default_download_dir" && !m.SettingsIsEditing {
		valueLabel = "[Tab] Browse: "
	}

	valueLabelStyle := lipgloss.NewStyle().Foreground(colors.NeonCyan).Bold(true)
	valueContentStyle := lipgloss.NewStyle().Foreground(colors.White)
	valueDisplay := valueLabelStyle.Render(valueLabel) + valueContentStyle.Render(valueStr)
	valueDisplay = lipgloss.NewStyle().Width(innerWidth).MaxWidth(innerWidth).Render(valueDisplay)

	divider := lipgloss.NewStyle().
		Foreground(colors.Gray).
		Render(strings.Repeat("─", innerWidth))

	descDisplay := lipgloss.NewStyle().
		Foreground(colors.LightGray).
		Width(innerWidth).
		MaxWidth(innerWidth).
		Render(meta.Description)

	detail := lipgloss.JoinVertical(lipgloss.Left,
		valueDisplay,
		"",
		divider,
		"",
		descDisplay,
	)

	return formatSettingsBlock(detail, innerWidth, rows)
}

func (m RootModel) renderSettingsTwoColumn(settingsMeta []config.SettingMeta, selectedRow int, settingsValues map[string]interface{}, modalWidth, bodyHeight int) string {
	leftWidth := 32
	minRightWidth := 22
	if modalWidth-leftWidth-8 < minRightWidth {
		leftWidth = modalWidth - minRightWidth - 8
	}
	if leftWidth < 16 {
		leftWidth = 16
	}

	rightWidth := modalWidth - leftWidth - 8
	if rightWidth < minRightWidth {
		rightWidth = minRightWidth
		if modalWidth-rightWidth-8 > 16 {
			leftWidth = modalWidth - rightWidth - 8
		}
	}

	if leftWidth < 12 || rightWidth < 14 {
		return m.renderSettingsCompact(settingsMeta, selectedRow, settingsValues, modalWidth, bodyHeight)
	}

	listRows := bodyHeight - 4
	if listRows < 1 {
		listRows = 1
	}
	listContent := renderSettingsListViewport(settingsMeta, selectedRow, listRows, leftWidth-4)
	listBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.Gray).
		Width(leftWidth).
		Padding(1, 1).
		Render(listContent)

	if m.SettingsIsEditing {
		m.updateSettingsInputWidthForViewport()
	}

	rightRows := bodyHeight - 2
	if rightRows < 1 {
		rightRows = 1
	}
	rightContent := m.renderSettingsDetailBlock(settingsMeta, selectedRow, settingsValues, rightWidth-4, rightRows)
	rightBox := lipgloss.NewStyle().
		Width(rightWidth).
		Padding(1, 2).
		Render(rightContent)

	dividerHeight := max(lipgloss.Height(listBox), lipgloss.Height(rightBox))
	if dividerHeight < 1 {
		dividerHeight = 1
	}
	divider := lipgloss.NewStyle().
		Foreground(colors.Gray).
		Render(strings.Repeat("│\n", dividerHeight-1) + "│")

	content := lipgloss.JoinHorizontal(lipgloss.Top, listBox, divider, rightBox)
	return formatSettingsBlock(content, modalWidth-2, bodyHeight)
}

func (m RootModel) renderSettingsCompact(settingsMeta []config.SettingMeta, selectedRow int, settingsValues map[string]interface{}, modalWidth, bodyHeight int) string {
	innerWidth := modalWidth - 2
	if innerWidth < 1 {
		innerWidth = 1
	}

	if m.SettingsIsEditing {
		m.updateSettingsInputWidthForViewport()
	}

	listRows := bodyHeight / 2
	if listRows < 1 {
		listRows = 1
	}

	detailRows := bodyHeight - listRows - 1
	if detailRows < 1 {
		detailRows = 1
		listRows = bodyHeight - detailRows
		if listRows < 1 {
			listRows = 1
		}
	}

	list := renderSettingsListViewport(settingsMeta, selectedRow, listRows, innerWidth)
	detail := m.renderSettingsDetailBlock(settingsMeta, selectedRow, settingsValues, innerWidth, detailRows)
	divider := lipgloss.NewStyle().Foreground(colors.Gray).Render(strings.Repeat("─", innerWidth))

	content := lipgloss.JoinVertical(lipgloss.Left,
		list,
		divider,
		detail,
	)

	return formatSettingsBlock(content, innerWidth, bodyHeight)
}

func (m *RootModel) normalizeSettingsSelection() {
	categories := config.CategoryOrder()
	if len(categories) == 0 {
		m.SettingsActiveTab = 0
		m.SettingsSelectedRow = 0
		if m.SettingsIsEditing {
			m.SettingsIsEditing = false
			m.SettingsInput.Blur()
		}
		return
	}

	if m.SettingsActiveTab < 0 {
		m.SettingsActiveTab = 0
	}
	if m.SettingsActiveTab >= len(categories) {
		m.SettingsActiveTab = len(categories) - 1
	}

	settingsMap := config.GetSettingsMetadata()
	settingsList := settingsMap[categories[m.SettingsActiveTab]]
	if len(settingsList) == 0 {
		m.SettingsSelectedRow = 0
		if m.SettingsIsEditing {
			m.SettingsIsEditing = false
			m.SettingsInput.Blur()
		}
		return
	}

	if m.SettingsSelectedRow < 0 {
		m.SettingsSelectedRow = 0
	}
	if m.SettingsSelectedRow >= len(settingsList) {
		m.SettingsSelectedRow = len(settingsList) - 1
	}
}

func (m *RootModel) updateSettingsInputWidthForViewport() {
	modalWidth, _ := settingsModalDimensions(m.width, m.height)

	var targetWidth int
	if modalWidth >= 72 {
		leftWidth := 32
		minRightWidth := 22
		if modalWidth-leftWidth-8 < minRightWidth {
			leftWidth = modalWidth - minRightWidth - 8
		}
		if leftWidth < 16 {
			leftWidth = 16
		}
		rightWidth := modalWidth - leftWidth - 8
		targetWidth = rightWidth - 10
	} else {
		targetWidth = modalWidth - 16
	}

	if targetWidth < 8 {
		targetWidth = 8
	}
	if targetWidth > 48 {
		targetWidth = 48
	}

	m.SettingsInput.SetWidth(targetWidth)
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
		values["proxy_url"] = m.Settings.Network.ProxyURL
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
	case "proxy_url":
		m.Settings.Network.ProxyURL = value
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
		case "proxy_url":
			m.Settings.Network.ProxyURL = defaults.Network.ProxyURL
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
