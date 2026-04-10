package tui

// GetHeaderHeight returns the appropriate header height based on terminal height
func GetHeaderHeight(termHeight int) int {
	if termHeight < ShortTermHeightThreshold {
		return HeaderHeightMin
	}
	return HeaderHeightMax
}

// GetMinGraphHeight returns the minimum graph height based on terminal height
func GetMinGraphHeight(termHeight int) int {
	if termHeight < ShortTermHeightThreshold {
		return MinGraphHeightShort
	}
	return MinGraphHeight
}

// GetSettingsDimensions calculates dimensions for settings/category modals
func GetSettingsDimensions(termWidth, termHeight int) (int, int) {
	width := int(float64(termWidth) * SettingsWidthRatio)
	if width < MinSettingsWidth {
		width = MinSettingsWidth
	}
	if width > MaxSettingsWidth {
		width = MaxSettingsWidth
	}

	maxWidth := termWidth - (WindowStyle.GetHorizontalFrameSize() * 2)
	if maxWidth < 1 {
		maxWidth = 1
	}
	if width > maxWidth {
		width = maxWidth
	}

	height := MinSettingsHeight
	maxHeight := termHeight - (WindowStyle.GetVerticalFrameSize() * 2) - 4 // fallback padding
	if maxHeight < 1 {
		maxHeight = 1
	}
	if height > maxHeight {
		height = maxHeight
	}

	return width, height
}

// GetListWidth calculates the list width based on available width
func GetListWidth(availableWidth int) int {
	leftWidth := int(float64(availableWidth) * ListWidthRatio)
	
	// Determine right column viability
	rightWidth := availableWidth - leftWidth
	if rightWidth < MinRightColumnWidth {
		return availableWidth
	}
	return leftWidth
}

// IsShortTerminal returns true if the terminal height is below the threshold
func IsShortTerminal(height int) bool {
	return height < ShortTermHeightThreshold
}

// GetGraphAreaDimensions calculates dimensions for the graph area
func GetGraphAreaDimensions(rightWidth int, isStatsHidden bool) (int, int) {
	axisWidth := GraphAxisWidth
	
	if isStatsHidden {
		// No stats box — graph gets almost full width
		graphAreaWidth := rightWidth - axisWidth - (BoxStyle.GetHorizontalFrameSize() * 5)
		if graphAreaWidth < 10 {
			graphAreaWidth = 10
		}
		return graphAreaWidth, axisWidth
	}
	
	// Graph takes remaining width after stats box
	graphAreaWidth := rightWidth - GraphStatsWidth - axisWidth - (BoxStyle.GetHorizontalFrameSize() * 3)
	if graphAreaWidth < 10 {
		graphAreaWidth = 10
	}
	return graphAreaWidth, axisWidth
}
