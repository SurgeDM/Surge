package tui

import (
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/SurgeDM/Surge/internal/tui/components"
)

func TestThemeSwitchRefreshesExistingUI(t *testing.T) {
	InitializeTUI()
	t.Cleanup(func() { colors.LoadTheme("", true) })
	m := InitialRootModel(1701, "test", nil, nil, config.DefaultSettings(), false)
	m.width, m.height = 160, 48
	m.list.SetSize(70, 25)
	m.logViewport.SetWidth(90)
	d := NewDownloadModel("theme-test", "https://example.com/file.zip", "file.zip", 100)
	d.started, d.Downloaded, d.Speed = true, 45, 1024
	d.progress.SetPercent(0.45)
	m.downloads = []*DownloadModel{d}
	m.activeTab = TabActive
	m.UpdateListItems()
	m.addStyledLogEntry(logError, "A download failed")
	m.addStyledLogEntry(logStarted, "A download started")
	m.addStyledLogEntry(logComplete, "A download completed")
	m.addStyledLogEntry(logPaused, "A download paused")
	m.graphRenderer = NewGraphRenderer()
	m.graphRenderer.Render([]float64{1, 2, 3}, 20, 5, 3, false)
	m.inputs[0].SetValue("https://example.com/file.zip")
	m.inputs[0].Focus()
	m.renderHeaderBox(40, 8) // Populate the logo cache before switching.

	assertColor := func(name string, got, want color.Color) {
		t.Helper()
		if colorToRGB(got) != colorToRGB(want) {
			t.Errorf("%s retained a stale color: got %v, want %v", name, got, want)
		}
	}
	for _, theme := range []string{"", "surge", "nord", "catppuccin", "gruvbox"} {
		for _, mode := range []int{config.ThemeLight, config.ThemeDark, config.ThemeLight} {
			name := theme
			if name == "" {
				name = "default"
			}
			t.Run(name+"/"+map[int]string{config.ThemeLight: "light", config.ThemeDark: "dark"}[mode], func(t *testing.T) {
				path := ""
				if theme != "" {
					path = filepath.Join("..", "..", "themes", theme+".toml")
				}
				m.ApplyTheme(mode, path)
				if d.progress.Percent() != 0.45 {
					t.Fatalf("theme switch reset progress: %v", d.progress.Percent())
				}
				if m.inputs[0].Value() != "https://example.com/file.zip" || !m.inputs[0].Focused() {
					t.Fatal("theme switch changed input content or focus")
				}
				assertColor("progress", d.progress.FullColor, colors.Pink())
				assertColor("empty progress", d.progress.EmptyColor, colors.Gray())
				assertColor("file picker", m.filepicker.Styles.Directory.GetForeground(), colors.Blue())
				assertColor("disabled file picker", m.filepicker.Styles.DisabledCursor.GetForeground(), colors.Gray())
				assertColor("help separator", m.help.Styles.ShortSeparator.GetForeground(), colors.Gray())
				inputs := append([]textinput.Model{}, m.inputs...)
				inputs = append(inputs, m.SettingsInput, m.searchInput, m.helpSearchInput, m.settingsSearchInput, m.urlUpdateInput)
				inputs = append(inputs, m.catMgrInputs[:]...)
				for _, input := range inputs {
					styles := input.Styles()
					assertColor("focused input", styles.Focused.Text.GetForeground(), colors.Foreground())
					assertColor("blurred input", styles.Blurred.Text.GetForeground(), colors.Foreground())
					assertColor("placeholder", styles.Focused.Placeholder.GetForeground(), colors.LightGray())
					assertColor("cursor", styles.Cursor.Color, colors.Pink())
				}
				for i, style := range []lipgloss.Style{LogStyleError, LogStyleStarted, LogStyleComplete, LogStylePaused} {
					if !strings.Contains(m.logViewport.View(), style.Render(m.logEntries[i])) {
						t.Fatal("existing log message did not adopt the active palette")
					}
				}
				if m.logoCache != "" || m.graphRenderer.baseGrid != nil {
					t.Fatal("theme switch did not invalidate rendered caches")
				}
				if m.list.Paginator.ActiveDot != lipgloss.NewStyle().Foreground(colors.Pink()).Render("\u2022") {
					t.Fatal("pagination retained its default colors")
				}
				for state := DashboardState; state <= RemoveConfirmState; state++ {
					m.state = state
					view := m.View()
					assertColor("view background", view.BackgroundColor, colors.Background())
					assertColor("view foreground", view.ForegroundColor, colors.Foreground())
					if view.Content == "" || lipgloss.Width(view.Content) > m.width || lipgloss.Height(view.Content) > m.height {
						t.Fatalf("state %d produced an empty or oversized themed view", state)
					}
				}
				// Also exercise cached status colors after changing a named theme.
				if !strings.Contains(components.StatusError.Render(), lipgloss.NewStyle().Foreground(colors.StateError()).Render("\u2716 Error")) {
					t.Fatal("status cache retained its old palette")
				}
			})
		}
	}
	for _, dark := range []bool{false, true} {
		m.InitialDarkBackground = dark
		m.ApplyTheme(config.ThemeAdaptive, filepath.Join("..", "..", "themes", "nord.toml"))
		if colors.IsDarkMode() != dark {
			t.Fatal("system theme ignored the detected terminal background")
		}
	}
}
