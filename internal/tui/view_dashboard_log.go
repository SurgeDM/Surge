package tui

import (
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

// renderLogBox returns the full Activity Log box with borders and title.
func (m *RootModel) renderLogBox(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	innerContent := m.logViewport.View()

	logBorderColor := colors.Gray
	if m.logFocused {
		logBorderColor = colors.NeonPink
	}

	return renderBtopBox(PaneTitleStyle.Render(" Activity Log "), "", innerContent, width, height, logBorderColor)
}
