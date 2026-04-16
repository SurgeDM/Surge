package tui

import (
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

// renderLogBox returns the full Activity Log box with borders and title.
func (m *RootModel) renderLogBox(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	contentWidth := width - 2
	contentHeight := height - 2

	if contentWidth < 0 {
		contentWidth = 0
	}
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Update viewport dimensions (content and padding are handled in helpers.go)
	m.logViewport.SetWidth(contentWidth)
	m.logViewport.SetHeight(contentHeight)

	innerContent := m.logViewport.View()

	logBorderColor := colors.Gray
	if m.logFocused {
		logBorderColor = colors.NeonPink
	}

	return renderBtopBox(PaneTitleStyle.Render(" Activity Log "), "", innerContent, width, height, logBorderColor)
}
