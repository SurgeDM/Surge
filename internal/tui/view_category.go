package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

// viewCategoryManager renders the category management screen.
func (m RootModel) viewCategoryManager() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	// Modal sizing
	width := int(float64(m.width) * 0.70)
	if width < 90 {
		width = 90
	}
	if width > 130 {
		width = 130
	}
	height := 26
	if m.width < width+4 {
		width = m.width - 4
	}
	if m.height < height+4 {
		height = m.height - 4
	}
	if width < 40 || height < 10 {
		content := lipgloss.NewStyle().
			Padding(1, 2).
			Foreground(colors.LightGray).
			Render("Terminal too small for category manager")
		box := renderBtopBox(PaneTitleStyle.Render(" Category Manager "), "", content, width, height, colors.NeonPurple)
		return m.renderModalWithOverlay(box)
	}

	cats := m.Settings.General.Categories

	// === TOGGLE BAR ===
	enabledStr := "OFF"
	enabledColor := colors.Gray
	if m.Settings.General.CategoryEnabled {
		enabledStr = "ON"
		enabledColor = colors.StateDownloading
	}
	toggleStyle := lipgloss.NewStyle().Foreground(enabledColor).Bold(true)
	toggleLine := lipgloss.NewStyle().Foreground(colors.LightGray).Render("  Auto-Sort Downloads: ") +
		toggleStyle.Render(enabledStr) +
		lipgloss.NewStyle().Foreground(colors.Gray).Render("  (t to toggle)")

	// === LEFT PANE: Category List ===
	leftWidth := 26
	if width-leftWidth-8 < 20 {
		leftWidth = width - 28
	}
	if leftWidth < 14 {
		leftWidth = 14
	}
	rightWidth := width - leftWidth - 8

	var listLines []string
	for i, cat := range cats {
		line := cat.Name
		if i == m.catMgrCursor && !m.catMgrEditing {
			style := lipgloss.NewStyle().Foreground(colors.NeonPurple).Bold(true)
			line = style.Render("▸ " + line)
		} else {
			style := lipgloss.NewStyle().Foreground(colors.LightGray)
			line = style.Render("  " + line)
		}
		listLines = append(listLines, line)
	}

	// "+ Add Category" row
	addLine := lipgloss.NewStyle().Foreground(colors.NeonCyan).Render("  + Add Category")
	if m.catMgrCursor == len(cats) && !m.catMgrEditing {
		addLine = lipgloss.NewStyle().Foreground(colors.NeonCyan).Bold(true).Render("▸ + Add Category")
	}
	listLines = append(listLines, addLine)

	listContent := lipgloss.JoinVertical(lipgloss.Left, listLines...)
	listBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.Gray).
		Width(leftWidth).
		Padding(1, 1).
		Render(listContent)

	// === RIGHT PANE: Details / Edit ===
	var rightContent string

	if m.catMgrEditing {
		// Edit mode with text inputs
		fieldLabels := []string{"Name:", "Description:", "Pattern:", "Path:"}
		var fieldLines []string
		for i, label := range fieldLabels {
			labelStyle := lipgloss.NewStyle().Foreground(colors.NeonCyan).Bold(true)
			var valueStr string
			if i == m.catMgrEditField {
				valueStr = m.catMgrInputs[i].View()
			} else {
				valStyle := lipgloss.NewStyle().Foreground(colors.White)
				valueStr = valStyle.Render(m.catMgrInputs[i].Value())
			}
			fieldLines = append(fieldLines, labelStyle.Render(label))
			fieldLines = append(fieldLines, "  "+valueStr)
			if i < len(fieldLabels)-1 {
				fieldLines = append(fieldLines, "")
			}
		}

		editHint := lipgloss.NewStyle().Foreground(colors.Gray).Render(
			"tab: next field  enter: save  esc: cancel")
		fieldLines = append(fieldLines, "", editHint)

		rightContent = lipgloss.JoinVertical(lipgloss.Left, fieldLines...)
	} else if m.catMgrCursor < len(cats) {
		// View mode - show selected category details
		cat := cats[m.catMgrCursor]
		labelStyle := lipgloss.NewStyle().Foreground(colors.NeonCyan).Bold(true)
		valueStyle := lipgloss.NewStyle().Foreground(colors.White)
		dimStyle := lipgloss.NewStyle().Foreground(colors.Gray)

		dividerWidth := rightWidth - 4
		if dividerWidth < 1 {
			dividerWidth = 1
		}
		divider := dimStyle.Render(strings.Repeat("─", dividerWidth))

		rightContent = lipgloss.JoinVertical(lipgloss.Left,
			labelStyle.Render("Name: ")+valueStyle.Render(cat.Name),
			"",
			labelStyle.Render("Description:"),
			valueStyle.Width(rightWidth-4).Render(cat.Description),
			"",
			divider,
			"",
			labelStyle.Render("Pattern (Regex):"),
			valueStyle.Width(rightWidth-4).Render(cat.Pattern),
			"",
			labelStyle.Render("Path:"),
			valueStyle.Width(rightWidth-4).Render(cat.Path),
		)
	} else {
		// On "+ Add Category" row
		rightContent = lipgloss.NewStyle().Foreground(colors.Gray).
			Width(rightWidth - 4).
			Render("Press Enter to create a new category\nor 'a' to add.")
	}

	rightBox := lipgloss.NewStyle().
		Width(rightWidth).
		Padding(1, 2).
		Render(rightContent)

	// === VERTICAL DIVIDER ===
	listBoxHeight := lipgloss.Height(listBox)
	dividerStyle := lipgloss.NewStyle().Foreground(colors.Gray)
	if listBoxHeight < 1 {
		listBoxHeight = 1
	}
	divider := dividerStyle.Render(strings.Repeat("│\n", listBoxHeight-1) + "│")

	// === COMBINE ===
	content := lipgloss.JoinHorizontal(lipgloss.Top, listBox, divider, rightBox)

	// === HELP ===
	helpStyle := lipgloss.NewStyle().
		Foreground(colors.Gray).
		Width(width - 6).
		Align(lipgloss.Center)
	helpText := helpStyle.Render(m.help.View(m.keys.CategoryMgr))

	// === INFO LINE ===
	catCount := fmt.Sprintf("%d categories", len(cats))
	infoLine := lipgloss.NewStyle().Foreground(colors.Gray).Render("  " + catCount)

	// Layout
	toggleBarHeight := lipgloss.Height(toggleLine)
	contentHeight := lipgloss.Height(content)
	helpHeight := lipgloss.Height(helpText)
	infoHeight := lipgloss.Height(infoLine)

	innerHeight := height - 2
	usedHeight := 1 + toggleBarHeight + 1 + infoHeight + contentHeight + helpHeight
	paddingLines := innerHeight - usedHeight
	if paddingLines < 0 {
		paddingLines = 0
	}
	padding := strings.Repeat("\n", paddingLines)

	fullContent := lipgloss.JoinVertical(lipgloss.Left,
		"",
		toggleLine,
		infoLine,
		"",
		content,
		padding+helpText,
	)

	box := renderBtopBox(PaneTitleStyle.Render(" Category Manager "), "", fullContent, width, height, colors.NeonPurple)
	return m.renderModalWithOverlay(box)
}
