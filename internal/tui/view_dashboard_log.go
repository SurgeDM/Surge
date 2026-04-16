package tui

import (
	"strings"

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

	m.logViewport.SetWidth(contentWidth)
	m.logViewport.SetHeight(contentHeight)

	// Pad log entries to bottom-align them conditionally based on current viewport height
	var paddedEntries []string
	if len(m.logEntries) < contentHeight {
		paddedEntries = make([]string, contentHeight-len(m.logEntries))
		paddedEntries = append(paddedEntries, m.logEntries...)
	} else {
		paddedEntries = m.logEntries
	}

	m.logViewport.SetContent(strings.Join(paddedEntries, "\n"))
	m.logViewport.GotoBottom()

	innerContent := m.logViewport.View()

	logBorderColor := colors.Gray
	if m.logFocused {
		logBorderColor = colors.NeonPink
	}

	return renderBtopBox(PaneTitleStyle.Render(" Activity Log "), "", innerContent, width, height, logBorderColor)
}
