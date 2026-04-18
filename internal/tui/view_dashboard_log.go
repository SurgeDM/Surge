package tui

import (
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/SurgeDM/Surge/internal/tui/components"
)

// renderLogBox returns the full Activity Log box with borders and title.
func (m *RootModel) renderLogBox(width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}

	var innerContent string
	if len(m.logEntries) == 0 {
		innerContent = renderEmptyMessage(width-components.BorderFrameWidth, height-components.BorderFrameHeight, "Activity log is empty")
	} else {
		innerContent = m.logViewport.View()
	}

	logBorderColor := colors.Gray()
	if m.logFocused {
		logBorderColor = colors.Pink()
	}

	return renderBtopBox(PaneTitleStyle.Render(" Activity Log "), "", innerContent, width, height, logBorderColor)
}
