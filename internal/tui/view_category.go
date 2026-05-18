package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/SurgeDM/Surge/internal/utils"
)

// viewCategoryManager renders the category management screen.
func (m RootModel) viewCategoryManager() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	width, height := GetSettingsDimensions(m.width, m.height)
	if width < MinSettingsWidth || height < 10 { // Rendering floor for modals
		content := lipgloss.NewStyle().
			Padding(DefaultPaddingY, DefaultPaddingX*2).
			Foreground(colors.LightGray()).
			Render("Terminal too small for category manager")
		box := renderBtopBox(PaneTitleStyle.Render(" Category Manager "), "", content, width, height, colors.Magenta())
		return m.renderModalWithOverlay(box)
	}

	cats := m.Settings.Categories.Categories
	cursor := m.catMgrCursor
	if m.catMgrEditing {
		if len(cats) == 0 {
			cursor = 0
		} else {
			if cursor < 0 {
				cursor = 0
			}
			if cursor >= len(cats) {
				cursor = len(cats) - 1
			}
		}
	} else {
		if cursor < 0 {
			cursor = 0
		}
		if cursor > len(cats) {
			cursor = len(cats)
		}
	}

	// === TOGGLE BAR ===
	enabledStr := "OFF"
	enabledColor := colors.Gray()
	if m.Settings.Categories.CategoryEnabled {
		enabledStr = "ON"
		enabledColor = colors.StateDownloading()
	}
	toggleStyle := lipgloss.NewStyle().Foreground(enabledColor).Bold(true)
	toggleLine := lipgloss.NewStyle().Foreground(colors.LightGray()).Render("  Auto-Sort Downloads: ") +
		toggleStyle.Render(enabledStr) +
		lipgloss.NewStyle().Foreground(colors.Gray()).Render("  (t to toggle)")
	if width < MinGraphStatsWidth {
		toggleLine = lipgloss.NewStyle().Foreground(colors.LightGray()).Render("  Auto-Sort: ") +
			toggleStyle.Render(enabledStr) +
			lipgloss.NewStyle().Foreground(colors.Gray()).Render("  (t)")
	}

	helpText := m.renderCategoryHelp(width - (ProgressBarWidthOffset + HeaderWidthOffset))
	catCount := fmt.Sprintf("%d categories", len(cats))
	infoLine := lipgloss.NewStyle().Foreground(colors.Gray()).Render("  " + catCount)

	innerHeight := height - BoxStyle.GetVerticalFrameSize()
	toggleBarHeight := lipgloss.Height(toggleLine)
	infoHeight := lipgloss.Height(infoLine)
	helpHeight := lipgloss.Height(helpText)
	bodyHeight := innerHeight - toggleBarHeight - infoHeight - helpHeight - LayoutGapStyle.GetVerticalFrameSize()
	if bodyHeight < 3 {
		bodyHeight = 3
	}

	var errorLine string
	if m.catMgrError != "" {
		errorLine = lipgloss.NewStyle().
			Foreground(colors.StateError()).
			Bold(true).
			Render("  \u2716 " + m.catMgrError)
	}

	var content string
	if width >= 76 && bodyHeight >= 9 {
		content = m.renderCategoryTwoColumn(cats, cursor, width, bodyHeight)
	} else {
		content = m.renderCategoryCompact(cats, cursor, width, bodyHeight)
	}

	contentHeight := lipgloss.Height(content)
	errorHeight := lipgloss.Height(errorLine)
	usedHeight := toggleBarHeight + infoHeight + errorHeight + LayoutGapStyle.GetVerticalFrameSize() + contentHeight + helpHeight
	paddingLines := innerHeight - usedHeight
	if paddingLines < 0 {
		paddingLines = 0
	}
	padding := strings.Repeat("\n", paddingLines)

	fullContent := lipgloss.JoinVertical(lipgloss.Left,
		toggleLine,
		infoLine,
		errorLine,
		"",
		content,
		padding+helpText,
	)

	box := renderBtopBox(PaneTitleStyle.Render(" Category Manager "), "", fullContent, width, height, colors.Magenta())
	return m.renderModalWithOverlay(box)
}

func (m RootModel) renderCategoryHelp(width int) string {
	if width < 1 {
		width = 1
	}

	helpText := m.help.View(m.keys.CategoryMgr)
	if width < MinGraphStatsWidth-2 {
		helpText = "esc: save/close  enter: edit/save  del: remove"
	}
	if width < MinTermWidth+3 {
		helpText = "esc close | enter edit | del rm"
	}

	return lipgloss.NewStyle().
		Foreground(colors.Gray()).
		Width(width).
		Align(lipgloss.Center).
		Render(helpText)
}

func renderCategoryListViewport(cats []config.Category, cursor int, editing bool, rows, innerWidth int) string {
	if rows < 1 {
		rows = 1
	}
	if innerWidth < 1 {
		innerWidth = 1
	}

	totalRows := len(cats) + 1 // + Add row
	if totalRows < 1 {
		totalRows = 1
	}

	if cursor < 0 {
		cursor = 0
	}
	if cursor >= totalRows {
		cursor = totalRows - 1
	}

	start := 0
	if cursor >= rows {
		start = cursor - rows + 1
	}
	maxStart := totalRows - rows
	if maxStart < 0 {
		maxStart = 0
	}
	if start > maxStart {
		start = maxStart
	}

	lines := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		idx := start + i
		if idx >= totalRows {
			lines = append(lines, "")
			continue
		}

		if idx < len(cats) {
			label := strings.TrimSpace(cats[idx].Name)
			if label == "" {
				label = "(Unnamed Category)"
			}

			prefix := "  "
			style := lipgloss.NewStyle().Foreground(colors.LightGray())
			if idx == cursor && !editing {
				prefix = "\u25b8 "
				style = lipgloss.NewStyle().Foreground(colors.Magenta()).Bold(true)
			}
			lines = append(lines, style.Width(innerWidth).MaxWidth(innerWidth).Render(prefix+label))
			continue
		}

		addPrefix := "  "
		addStyle := lipgloss.NewStyle().Foreground(colors.Cyan())
		if idx == cursor && !editing {
			addPrefix = "\u25b8 "
			addStyle = lipgloss.NewStyle().Foreground(colors.Cyan()).Bold(true)
		}
		lines = append(lines, addStyle.Width(innerWidth).MaxWidth(innerWidth).Render(addPrefix+"+ Add Category"))
	}

	return strings.Join(lines, "\n")
}

func (m RootModel) renderCategoryDetailView(cats []config.Category, cursor, innerWidth, rows int) string {
	if innerWidth < 1 {
		innerWidth = 1
	}
	if rows < 1 {
		rows = 1
	}

	if cursor >= len(cats) || len(cats) == 0 {
		msg := lipgloss.NewStyle().
			Foreground(colors.Gray()).
			Width(innerWidth).
			MaxWidth(innerWidth).
			Render("Press Enter to create a new category\nor press 'a' to add.")
		return formatSettingsBlock(msg, innerWidth, rows)
	}

	cat := cats[cursor]
	labelStyle := lipgloss.NewStyle().Foreground(colors.Cyan()).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(colors.White())
	dimStyle := lipgloss.NewStyle().Foreground(colors.Gray())
	divider := dimStyle.Render(strings.Repeat("\u2500", innerWidth))

	content := lipgloss.JoinVertical(lipgloss.Left,
		labelStyle.Render("Name: ")+valueStyle.Width(innerWidth-6).MaxWidth(innerWidth-6).Render(utils.TruncateTwoLines(cat.Name, innerWidth-6)),
		"",
		labelStyle.Render("Description:"),
		valueStyle.Width(innerWidth).MaxWidth(innerWidth).Render(utils.TruncateTwoLines(cat.Description, innerWidth)),
		"",
		divider,
		"",
		labelStyle.Render("Pattern (Regex):"),
		valueStyle.Width(innerWidth).MaxWidth(innerWidth).Render(utils.TruncateTwoLines(cat.Pattern, innerWidth)),
		"",
		labelStyle.Render("Path:"),
		valueStyle.Width(innerWidth).MaxWidth(innerWidth).Render(utils.TruncateTwoLines(cat.Path, innerWidth)),
	)

	return formatSettingsBlock(content, innerWidth, rows)
}

func (m RootModel) renderCategoryEditView(innerWidth, rows int) string {
	if innerWidth < 1 {
		innerWidth = 1
	}
	if rows < 1 {
		rows = 1
	}

	fieldLabels := []string{"Name:", "Description:", "Pattern:", "Path:"}
	var fieldLines []string
	for i, label := range fieldLabels {
		labelStyle := lipgloss.NewStyle().Foreground(colors.Cyan()).Bold(true)
		valueStyle := lipgloss.NewStyle().Foreground(colors.White())
		value := m.catMgrInputs[i].Value()
		if i == m.catMgrEditField {
			value = m.catMgrInputs[i].View()
		}
		fieldLines = append(fieldLines, labelStyle.Width(innerWidth).MaxWidth(innerWidth).Render(label))
		fieldLines = append(fieldLines, valueStyle.Width(innerWidth).MaxWidth(innerWidth).Render("  "+value))
		if i < len(fieldLabels)-1 {
			fieldLines = append(fieldLines, "")
		}
	}

	hint := lipgloss.NewStyle().
		Foreground(colors.Gray()).
		Width(innerWidth).
		MaxWidth(innerWidth).
		Render("tab: next field  enter: save  esc: cancel")
	fieldLines = append(fieldLines, "", hint)

	return formatSettingsBlock(strings.Join(fieldLines, "\n"), innerWidth, rows)
}

func (m RootModel) renderCategoryTwoColumn(cats []config.Category, cursor, modalWidth, bodyHeight int) string {
	leftWidth, rightWidth := CalculateTwoColumnWidths(modalWidth, 28, 24)

	if leftWidth < 14 || rightWidth < 16 {
		return m.renderCategoryCompact(cats, cursor, modalWidth, bodyHeight)
	}

	listBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.Gray()).
		Width(leftWidth).
		Padding(1, 1)

	listRows := bodyHeight - listBoxStyle.GetVerticalFrameSize()
	if listRows < 1 {
		listRows = 1
	}
	listContent := renderCategoryListViewport(cats, cursor, m.catMgrEditing, listRows, leftWidth-listBoxStyle.GetHorizontalFrameSize())
	listBox := listBoxStyle.Render(listContent)

	if m.catMgrEditing {
		m.updateCategoryInputWidthsForViewport()
	}

	rightBoxStyle := lipgloss.NewStyle().
		Width(rightWidth).
		Padding(1, 2)

	rightRows := bodyHeight - rightBoxStyle.GetVerticalFrameSize()
	if rightRows < 1 {
		rightRows = 1
	}

	var rightContent string
	if m.catMgrEditing {
		rightContent = m.renderCategoryEditView(rightWidth-rightBoxStyle.GetHorizontalFrameSize(), rightRows)
	} else {
		rightContent = m.renderCategoryDetailView(cats, cursor, rightWidth-rightBoxStyle.GetHorizontalFrameSize(), rightRows)
	}

	rightBox := rightBoxStyle.Render(rightContent)

	dividerHeight := max(lipgloss.Height(listBox), lipgloss.Height(rightBox))
	if dividerHeight < 1 {
		dividerHeight = 1
	}
	divider := lipgloss.NewStyle().Foreground(colors.Gray()).Render(strings.Repeat("\u2502\n", dividerHeight-1) + "\u2502")
	content := lipgloss.JoinHorizontal(lipgloss.Top, listBox, divider, rightBox)
	return formatSettingsBlock(content, modalWidth-BoxStyle.GetHorizontalFrameSize(), bodyHeight)
}

func (m RootModel) renderCategoryCompact(cats []config.Category, cursor, modalWidth, bodyHeight int) string {
	innerWidth := modalWidth - BoxStyle.GetHorizontalFrameSize()
	if innerWidth < 1 {
		innerWidth = 1
	}

	if m.catMgrEditing {
		m.updateCategoryInputWidthsForViewport()
	}

	listRows := bodyHeight / 2
	if listRows < 1 {
		listRows = 1
	}

	detailRows := bodyHeight - listRows - DividerHeight // line for the divider line
	if detailRows < 1 {
		detailRows = 1
		listRows = bodyHeight - detailRows
		if listRows < 1 {
			listRows = 1
		}
	}

	list := renderCategoryListViewport(cats, cursor, m.catMgrEditing, listRows, innerWidth)

	var detail string
	if m.catMgrEditing {
		detail = m.renderCategoryEditView(innerWidth, detailRows)
	} else {
		detail = m.renderCategoryDetailView(cats, cursor, innerWidth, detailRows)
	}

	divider := lipgloss.NewStyle().Foreground(colors.Gray()).Render(strings.Repeat("\u2500", innerWidth))
	content := lipgloss.JoinVertical(lipgloss.Left,
		list,
		divider,
		detail,
	)

	return formatSettingsBlock(content, innerWidth, bodyHeight)
}
