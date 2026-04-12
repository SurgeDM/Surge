package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/SurgeDM/Surge/internal/tui/colors"
)

func boolLabel(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
}

func (m RootModel) viewTransfer() string {
	width := 76
	if m.width > 0 && m.width < width {
		width = m.width
	}

	lines := []string{
		lipgloss.NewStyle().Foreground(colors.NeonCyan).Bold(true).Render("Data Transfer"),
		"",
		fmt.Sprintf("[e] Export bundle"),
		fmt.Sprintf("[i] Import bundle"),
		fmt.Sprintf("[p] Include partials: %s", boolLabel(m.transferIncludePartials)),
		fmt.Sprintf("[l] Include logs: %s", boolLabel(m.transferIncludeLogs)),
		fmt.Sprintf("[x] Replace on apply: %s", boolLabel(m.transferReplace)),
		fmt.Sprintf("[r] Import root: %s", m.transferRootDir),
	}

	if m.transferImportFile != "" {
		lines = append(lines, fmt.Sprintf("Bundle: %s", truncateString(m.transferImportFile, 58)))
	}
	if m.transferPreview != nil {
		lines = append(lines,
			"",
			lipgloss.NewStyle().Foreground(colors.NeonPink).Render("Preview"),
			fmt.Sprintf("Imports: %v", m.transferPreview.ImportsByStatus),
			fmt.Sprintf("Duplicates skipped: %d", m.transferPreview.DuplicatesSkipped),
			fmt.Sprintf("Renamed items: %d", m.transferPreview.RenamedItems),
			fmt.Sprintf("Downgraded to queue: %d", m.transferPreview.ResumableJobsDowngradedToQueue),
			"[a] Apply import",
		)
	}
	if strings.TrimSpace(m.transferStatus) != "" {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(colors.LightGray).Render(m.transferStatus))
	}

	body := strings.Join(lines, "\n")
	box := renderBtopBox(
		PaneTitleStyle.Render(" Data Transfer "),
		"",
		lipgloss.NewStyle().Padding(1, 2).Render(body),
		width,
		20,
		colors.NeonCyan,
	)
	return box
}

