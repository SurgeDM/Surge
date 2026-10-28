package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

func (m RootModel) categoryPickerOptions() []string {
	if m.categoryPickerAssign {
		return append([]string{"Uncategorized"}, config.CategoryNames(m.Settings.Categories.Categories)...)
	}
	return append([]string{"", "Uncategorized"}, config.CategoryNames(m.Settings.Categories.Categories)...)
}

func (m RootModel) categoryFilterPickerCursor() int {
	for i, option := range m.categoryPickerOptions() {
		if option == m.categoryFilter {
			return i
		}
	}
	return 0
}

func (m RootModel) updateCategoryPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	options := m.categoryPickerOptions()
	if key.Matches(msg, m.keys.Settings.Close) {
		m.categoryPickerTarget = ""
		m.state = DashboardState
		return m, nil
	}
	if key.Matches(msg, m.keys.Settings.Up) {
		m.categoryPickerCursor = (m.categoryPickerCursor - 1 + len(options)) % len(options)
		return m, nil
	}
	if key.Matches(msg, m.keys.Settings.Down) {
		m.categoryPickerCursor = (m.categoryPickerCursor + 1) % len(options)
		return m, nil
	}
	if key.Matches(msg, m.keys.Settings.Edit) {
		selected := options[m.categoryPickerCursor]
		if m.categoryPickerAssign {
			if d := m.FindDownloadByID(m.categoryPickerTarget); d != nil {
				category := selected
				if category == "Uncategorized" {
					category = ""
				}
				d.categoryOverride = &category
				label := selected
				m.addStyledLogEntry(logStarted, "Category: "+label)
				m.UpdateListItems()
			}
			m.categoryPickerTarget = ""
			m.state = DashboardState
			return m, nil
		}
		m.categoryFilter = selected
		label := m.categoryFilter
		if label == "" {
			label = "All"
		}
		m.addStyledLogEntry(logStarted, "Filter: "+label)
		m.UpdateListItems()
		m.state = DashboardState
	}
	return m, nil
}

type categoryPickerItem struct{ Label, Value string }

func (m RootModel) viewCategoryPicker() string {
	options := m.categoryPickerOptions()
	items := make([]categoryPickerItem, len(options))
	for i, option := range options {
		label, value := option, ""
		switch option {
		case "":
			label, value = "All Downloads", "Show every download"
		case "Uncategorized":
			if !m.categoryPickerAssign {
				value = "No matching category"
			}
		default:
			for _, category := range m.Settings.Categories.Categories {
				if category.Name == option {
					value = category.Description
					break
				}
			}
		}
		items[i] = categoryPickerItem{Label: label, Value: value}
	}
	w, h := GetDynamicModalDimensions(m.width, m.height, 36, 8, 88, len(options)+6)
	title := "Filter by Category"
	if m.categoryPickerAssign {
		title = "Assign Category"
	}
	innerWidth := max(1, w-6)
	visible := max(1, h-6)
	listWidth := min(24, max(12, innerWidth/3))
	detailWidth := max(1, innerWidth-listWidth-3)
	start := max(0, m.categoryPickerCursor-visible+1)
	start = min(start, max(0, len(items)-visible))
	var rows []string
	for i := start; i < min(len(items), start+visible); i++ {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(colors.LightGray())
		if i == m.categoryPickerCursor {
			prefix = "\u25b8 "
			style = style.Foreground(colors.Pink()).Bold(true)
		}
		rows = append(rows, style.Render(ansi.Truncate(prefix+items[i].Label, listWidth, "\u2026")))
	}
	list := lipgloss.NewStyle().Width(listWidth).Height(visible).Render(strings.Join(rows, "\n"))
	description := ""
	if m.categoryPickerCursor >= 0 && m.categoryPickerCursor < len(items) {
		description = items[m.categoryPickerCursor].Value
	}
	detail := lipgloss.NewStyle().Foreground(colors.White()).Width(detailWidth).MaxHeight(visible).Render(description)
	divider := lipgloss.NewStyle().Foreground(colors.Gray()).Render(strings.TrimSuffix(strings.Repeat(" \u2502 \n", visible), "\n"))
	panes := lipgloss.JoinHorizontal(lipgloss.Top, list, divider, detail)
	hint := m.keys.Settings.Up.Help().Key + "/" + m.keys.Settings.Down.Help().Key +
		" navigate   " + m.keys.Settings.Edit.Help().Key + " select   " + m.keys.Settings.Close.Help().Key + " close"
	footer := lipgloss.NewStyle().Foreground(colors.Gray()).Render(ansi.Truncate(hint, innerWidth, "\u2026"))
	content := lipgloss.NewStyle().Padding(1, 2).Render(lipgloss.JoinVertical(lipgloss.Left, panes, "", footer))
	return renderBtopBox(PaneTitleStyle.Render(" "+title+" "), "", content, w, h, colors.Magenta())
}
