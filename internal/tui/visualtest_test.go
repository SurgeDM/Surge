package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

var (
	visualTheme    = flag.String("theme", "", "Override the configured theme")
	visualLight    = flag.Bool("light", false, "Use light mode")
	visualDark     = flag.Bool("dark", false, "Use dark mode")
	visualSnapshot = flag.String("snapshot", "", "Write an ANSI snapshot to this path instead of opening the TUI")
	visualWidth    = flag.Int("width", VisualTestWidth, "Snapshot columns")
	visualHeight   = flag.Int("height", VisualTestHeight, "Snapshot rows")
	visualSelected = flag.String("select", "downloading", "Selected fixture: downloading, queued, paused, pausing, resuming, completed, error")
)

// TestVisualTUI is opt-in and lives only in the Go test binary. The wrapper
// script runs that binary directly so Bubble Tea retains the real terminal.
func TestVisualTUI(t *testing.T) {
	if os.Getenv("SURGE_VISUAL_TEST") != "1" {
		t.Skip("run ./scripts/visual-test.sh to inspect the real TUI")
	}
	settings, err := config.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if *visualTheme != "" {
		settings.General.ThemePath.Value = *visualTheme
	}
	if *visualLight && *visualDark {
		t.Fatal("choose either --light or --dark")
	}
	if *visualLight {
		settings.General.Theme.Value = config.ThemeLight
	}
	if *visualDark {
		settings.General.Theme.Value = config.ThemeDark
	}
	isolateVisualConfig(t, true)
	previousTestMode := IsTestMode
	if *visualSnapshot == "" {
		IsTestMode = false
	}
	t.Cleanup(func() { IsTestMode = previousTestMode })
	m := NewVisualTestModel(settings, "visual-test")
	states := []string{"downloading", "queued", "paused", "pausing", "resuming", "completed", "error"}
	selected := -1
	for i, state := range states {
		if state == *visualSelected {
			selected = i
			break
		}
	}
	if selected < 0 {
		t.Fatalf("unknown fixture %q", *visualSelected)
	}
	m.root.SelectedDownloadID = m.root.downloads[selected].ID
	m.root.UpdateListItems()
	if *visualSnapshot != "" {
		if *visualWidth < 1 || *visualHeight < 1 {
			t.Fatal("snapshot dimensions must be positive")
		}
		if err := os.WriteFile(*visualSnapshot, []byte(m.Snapshot(*visualWidth, *visualHeight)+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		t.Fatal(err)
	}
}

func isolateVisualConfig(t *testing.T, copyExisting bool) {
	t.Helper()
	source := config.GetSurgeDir()
	target := t.TempDir()
	if copyExisting {
		copyFile := func(source, target string) {
			t.Helper()
			data, err := os.ReadFile(source)
			if os.IsNotExist(err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		copyFile(filepath.Join(source, "keymap.json"), filepath.Join(target, "surge", "keymap.json"))
		entries, err := os.ReadDir(filepath.Join(source, "themes"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".toml") {
				copyFile(filepath.Join(source, "themes", entry.Name()), filepath.Join(target, "surge", "themes", entry.Name()))
			}
		}
	}
	t.Setenv("XDG_CONFIG_HOME", target)
	t.Setenv("XDG_STATE_HOME", target)
	t.Setenv("APPDATA", target)
}

func fixtureSettings(t *testing.T, theme string, dark bool) *config.Settings {
	t.Helper()
	settings := config.DefaultSettings()
	if theme != "" {
		path, err := filepath.Abs(filepath.Join("..", "..", "themes", theme+".toml"))
		if err != nil {
			t.Fatal(err)
		}
		settings.General.ThemePath.Value = path
	}
	settings.General.Theme.Value = config.ThemeLight
	if dark {
		settings.General.Theme.Value = config.ThemeDark
	}
	return settings
}

func TestVisualFixtureRendersAllDownloadsOnOneScreen(t *testing.T) {
	isolateVisualConfig(t, false)
	t.Cleanup(func() { colors.LoadTheme("", true) })
	for _, theme := range []string{"surge", "nord", "catppuccin", "gruvbox"} {
		for _, dark := range []bool{true, false} {
			m := NewVisualTestModel(fixtureSettings(t, theme, dark), "test")
			for _, size := range [][2]int{{140, 56}, {160, 56}, {200, 60}} {
				rendered := m.Snapshot(size[0], size[1])
				plain := stripANSI.ReplaceAllString(rendered, "")
				for _, label := range []string{"Downloading", "Queued", "Paused", "Pausing...", "Resuming...", "Completed", "Error", "Network Activity", "Top:", "Total:", "Chunk Map"} {
					if !strings.Contains(plain, label) {
						t.Fatalf("%s dark=%v %v: missing %q", theme, dark, size, label)
					}
				}
				if strings.Contains(plain, "\u25a0 Complete") || strings.Count(plain, "Downloading") > 2 {
					t.Fatal("fixture added a comparison grid or legend absent from production")
				}
				if lipgloss.Width(rendered) > size[0] || lipgloss.Height(rendered) > size[1] {
					t.Fatalf("fixture exceeds screen %v", size)
				}
			}
			if m.Snapshot(160, 56) != m.Snapshot(160, 56) {
				t.Fatal("fixture snapshot changed without input")
			}
			if m.root.Service != nil || m.root.Orchestrator != nil || m.root.SaveSettingsFunc != nil {
				t.Fatal("fixture enabled a live backend")
			}
		}
	}
}

func TestVisualFixtureMatchesProductionInitializationAndRendering(t *testing.T) {
	isolateVisualConfig(t, false)
	t.Cleanup(func() { colors.LoadTheme("", true) })
	for _, theme := range []string{"surge", "nord", "catppuccin", "gruvbox"} {
		settings := fixtureSettings(t, theme, true)
		fixture := NewVisualTestModel(settings, "test")
		production := InitialRootModel(0, "test", nil, nil, settings, false)
		seedVisualTestData(&production)
		for _, selected := range []int{0, 2, 5, 6} {
			for _, size := range [][2]int{{80, 24}, {160, 56}} {
				for _, root := range []*RootModel{&fixture.root, &production} {
					root.SelectedDownloadID = root.downloads[selected].ID
					root.UpdateListItems()
					updated, _ := root.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
					*root = updated.(RootModel)
					root.lastResizeTime = time.Time{}
				}
				expected, actual := production.View(), fixture.View()
				if actual.Content != expected.Content || colorToRGB(actual.BackgroundColor) != colorToRGB(expected.BackgroundColor) {
					t.Fatalf("%s selected=%d size=%v: visual test diverged from production TUI", theme, selected, size)
				}
			}
		}
	}
}

func TestVisualFixtureUsesNormalNavigationAndSettings(t *testing.T) {
	isolateVisualConfig(t, false)
	t.Cleanup(func() { colors.LoadTheme("", true) })
	m := NewVisualTestModel(fixtureSettings(t, "", true), "test")
	press := func(msg tea.KeyPressMsg) { t.Helper(); updated, _ := m.Update(msg); m = updated.(VisualTestModel) }
	press(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.root.list.Index() != 1 {
		t.Fatal("normal list navigation failed")
	}
	press(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if m.root.activeTab != TabQueued {
		t.Fatal("normal queued shortcut failed")
	}
	press(tea.KeyPressMsg{Code: '0', Text: "0"})
	if m.root.activeTab != TabAll || len(m.root.list.Items()) != 7 {
		t.Fatal("normal All shortcut failed")
	}
	press(tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.root.state != SettingsState {
		t.Fatal("normal settings did not open")
	}
	press(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.root.state != DashboardState {
		t.Fatal("normal settings did not close")
	}
	press(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	if m.root.state != QuitConfirmState {
		t.Fatal("normal quit dialog did not open")
	}
}

func TestVisualFixtureCannotChangeUserConfiguration(t *testing.T) {
	isolateVisualConfig(t, false)
	if err := config.SaveKeyMap(config.DefaultKeyMap()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(config.GetKeyMapConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	m := NewVisualTestModel(fixtureSettings(t, "", true), "test")
	m.root.Settings.General.Theme.Value = config.ThemeLight
	if err := m.root.persistSettings(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config.GetSettingsPath()); !os.IsNotExist(err) {
		t.Fatal("visual fixture persisted settings")
	}
	after, err := os.ReadFile(config.GetKeyMapConfigPath())
	if err != nil || string(before) != string(after) {
		t.Fatal("visual fixture unexpectedly changed its keymap")
	}
}
