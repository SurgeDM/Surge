package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func (m RootModel) updateHelpModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.helpSearchActive {
		switch msg.String() {
		case "esc", "Q", "ctrl+c":
			m.helpSearchActive = false
			m.helpSearchInput.Blur()
			m.helpSearchQuery = ""
			m.helpSearchInput.SetValue("")
			return m, nil
		case "enter":
			m.helpSearchActive = false
			m.helpSearchInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.helpSearchInput, cmd = m.helpSearchInput.Update(msg)
			m.helpSearchQuery = m.helpSearchInput.Value()
			return m, cmd
		}
	}
	if msg.String() == "/" || key.Matches(msg, m.keys.Dashboard.Search) {
		m.helpSearchActive = true
		m.helpSearchInput.Prompt = ""
		m.helpSearchInput.Placeholder = "Type to search..."
		m.helpSearchInput.SetWidth(30)
		return m, m.helpSearchInput.Focus()
	}
	if msg.String() == "esc" || key.Matches(msg, m.keys.Dashboard.ToggleHelp) {
		m.state = DashboardState
	}
	return m, nil
}

func (m RootModel) helpSearchTitle() string {
	if m.helpSearchActive {
		return " > " + m.helpSearchInput.View() + " [esc] Exit "
	}
	if m.helpSearchQuery != "" {
		return " > " + m.helpSearchQuery + " [" + m.keys.Dashboard.Search.Help().Key + "] Edit "
	}
	return " [" + m.keys.Dashboard.Search.Help().Key + "] Search "
}
