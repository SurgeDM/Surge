package tui

import (
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

func TestStatusLabelKeepsItsColorAfterStyledSpinner(t *testing.T) {
	InitializeTUI()
	t.Cleanup(func() { colors.LoadTheme("", true) })
	for _, theme := range []string{"", "surge", "nord", "catppuccin", "gruvbox"} {
		for _, dark := range []bool{false, true} {
			path := ""
			if theme != "" {
				path = filepath.Join("..", "..", "themes", theme+".toml")
			}
			colors.LoadTheme(path, dark)
			// The shared spinner has its own foreground and terminating ANSI reset.
			spinner := lipgloss.NewStyle().Foreground(colors.Pink()).Render("\u280b")
			for _, state := range []struct {
				name  string
				model DownloadModel
				color color.Color
			}{
				{"Pausing...", DownloadModel{pausing: true}, colors.StatePaused()},
				{"Resuming...", DownloadModel{started: true, resuming: true}, colors.StateDownloading()},
				{"Queued", DownloadModel{}, colors.StatePaused()},
			} {
				want := lipgloss.NewStyle().Foreground(state.color).Render("\u280b " + state.name)
				if got := getDownloadStatus(&state.model, spinner); got != want {
					t.Errorf("%s dark=%v %s: spinner reset escaped the status color: got %q want %q", theme, dark, state.name, got, want)
				}
				if got := (DownloadItem{download: &state.model, spinnerView: spinner}).Description(); !strings.HasPrefix(got, want) {
					t.Errorf("%s dark=%v %s: list and details do not use the same uninterrupted status color", theme, dark, state.name)
				}
			}
		}
	}
}

func TestProgressThemePreservesOriginalContinuousFill(t *testing.T) {
	t.Cleanup(func() { colors.LoadTheme("", true) })
	for _, dark := range []bool{false, true} {
		colors.LoadTheme("", dark)
		model := NewDownloadModel("progress", "", "file", 100)
		model.progress.SetWidth(40)
		// The original two-stop fill colors BOTH halves of each half-block cell.
		original := progress.New(progress.WithColors(colors.Pink(), colors.Pink()), progress.WithScaled(true))
		original.SetWidth(40)
		original.EmptyColor = colors.Gray()
		original.PercentageStyle = lipgloss.NewStyle().Foreground(colors.Foreground())
		for _, pct := range []float64{0.25, 0.5, 0.75, 1} {
			if got, want := model.progress.ViewAs(pct), original.ViewAs(pct); got != want {
				t.Fatalf("dark=%v percent=%v: theme helper changed the original continuous fill", dark, pct)
			}
		}
	}
}
