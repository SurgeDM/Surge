package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

func (m RootModel) updateSettingsSearch(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.settingsSearchActive {
		switch msg.String() {
		case "esc", "Q", "ctrl+c":
			m.settingsSearchActive = false
			m.settingsSearchInput.Blur()
			m.settingsSearchQuery = ""
			m.settingsSearchInput.SetValue("")
			return m, nil
		case "enter":
			m.settingsSearchActive = false
			m.settingsSearchInput.Blur()
			return m, nil
		default:
			var cmd tea.Cmd
			m.settingsSearchInput, cmd = m.settingsSearchInput.Update(msg)
			m.settingsSearchQuery = m.settingsSearchInput.Value()
			return m, cmd
		}
	}
	if msg.String() == "/" || key.Matches(msg, m.keys.Dashboard.Search) {
		m.settingsSearchActive = true
		m.settingsSearchInput.Prompt = ""
		m.settingsSearchInput.Placeholder = "Type to search..."
		m.settingsSearchInput.SetWidth(30)
		return m, m.settingsSearchInput.Focus()
	}
	return m, nil
}

func (m RootModel) settingsSearchTitle() string {
	if m.settingsSearchActive {
		return " " + lipgloss.NewStyle().Foreground(colors.Cyan()).Render("> ") + m.settingsSearchInput.View() + " [esc] Exit "
	}
	if m.settingsSearchQuery != "" {
		return " " + lipgloss.NewStyle().Foreground(colors.Cyan()).Render("> ") + m.settingsSearchQuery + " [" + m.keys.Dashboard.Search.Help().Key + "] Edit "
	}
	return " [" + m.keys.Dashboard.Search.Help().Key + "] Search "
}

// Resolve filtered rows back to their original category before reading or editing.
func (m RootModel) currentSettingsCategory() string {
	categories := config.CategoryOrder()
	if m.SettingsActiveTab < 0 || m.SettingsActiveTab >= len(categories) {
		return ""
	}
	if m.settingsSearchQuery == "" {
		return categories[m.SettingsActiveTab]
	}
	key := m.getCurrentSettingKey()
	unfiltered := m
	unfiltered.settingsSearchQuery = ""
	for _, category := range categories {
		for _, meta := range unfiltered.buildSettingsMetaForCategory(category) {
			if meta.Key == key {
				return category
			}
		}
	}
	return ""
}
