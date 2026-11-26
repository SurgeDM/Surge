package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

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

	var hint string
	logBorderColor := colors.Gray()
	if m.logFocused {
		logBorderColor = colors.Pink()
		if len(m.logEntries) > 0 {
			hint = PaneTitleStyle.Render(" Enter: view ")
		}
	}

	return renderBtopBox(PaneTitleStyle.Render(" Activity Log ["+m.keys.Dashboard.Log.Help().Key+"] "), hint, innerContent, width, height, logBorderColor)
}

func (m RootModel) updateActivityLog(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := m.keys.Dashboard
	switch {
	case key.Matches(msg, keys.LogClose):
		m.logFocused = false
	case len(m.logEntries) == 0:
		return m, nil
	case key.Matches(msg, keys.LogDown):
		m.logCursor = min(m.logCursor+1, len(m.logEntries)-1)
	case key.Matches(msg, keys.LogUp):
		m.logCursor = max(m.logCursor-1, 0)
	case key.Matches(msg, keys.LogTop):
		m.logCursor = 0
	case key.Matches(msg, keys.LogBottom):
		m.logCursor = len(m.logEntries) - 1
	case msg.String() == "enter":
		m.logMessage = m.logEntries[m.logCursor]
		m.logMessageViewport = viewport.New()
		m.resizeLogMessage()
		m.state = LogMessageState
		return m, nil
	}
	m.refreshLogViewportContent()
	return m, nil
}

func (m *RootModel) resizeLogMessage() {
	width, height := GetDynamicModalDimensions(m.width, m.height, 30, 5, 96, 18)
	width = max(1, width-components.BorderFrameWidth)
	wrapped := ansi.Hardwrap(m.logMessage, width, true)
	m.logMessageViewport.SetWidth(width)
	m.logMessageViewport.SetHeight(min(max(1, height-components.BorderFrameHeight), strings.Count(wrapped, "\n")+1))
	m.logMessageViewport.SetContent(wrapped)
}

func (m RootModel) updateLogMessage(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "enter" || key.Matches(msg, m.keys.Dashboard.LogClose) {
		m.state = DashboardState
		m.logMessage = ""
		m.logMessageViewport = viewport.Model{}
		return m, nil
	}
	var cmd tea.Cmd
	m.logMessageViewport, cmd = m.logMessageViewport.Update(msg)
	return m, cmd
}

func (m *RootModel) renderLogMessage() string {
	hint := m.keys.Dashboard.LogClose.Help().Key + ": close"
	if m.logMessageViewport.TotalLineCount() > m.logMessageViewport.Height() {
		hint = "\u2191/\u2193: scroll \u00b7 " + hint
	}
	return renderBtopBox(
		PaneTitleStyle.Render(" Activity Message "),
		PaneTitleStyle.Render(" "+hint+" "),
		m.logMessageViewport.View(),
		m.logMessageViewport.Width()+components.BorderFrameWidth,
		m.logMessageViewport.Height()+components.BorderFrameHeight,
		colors.Pink(),
	)
}
