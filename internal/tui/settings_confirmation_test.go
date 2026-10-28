package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
)

func TestSettingsFooterKeepsCategoryPaneHeightStable(t *testing.T) {
	for _, setting := range []string{"category_enabled", "category_0", "add_category"} {
		m := confirmationSettingsModel(t, "Categories", setting)
		if got := lipgloss.Height(m.renderSettingsHelp(150)); got != 2 {
			t.Errorf("%s footer height = %d, want 2", setting, got)
		}
	}
}

func confirmationSettingsModel(t *testing.T, category, setting string) RootModel {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	InitializeTUI()
	m := RootModel{Settings: config.DefaultSettings(), keys: config.DefaultKeyMap(), help: help.New(), state: SettingsState, SettingsFocusedPane: 1, width: 160, height: 48}
	m.lastConfigCheckTime = time.Now()
	m.Settings.Categories.Categories = []config.Category{{Name: "First", Pattern: `\.zip$`, Path: "/first"}, {Name: "Second", Pattern: `\.zip$`, Path: "/second"}}
	for i, name := range config.CategoryOrder() {
		if name == category {
			m.SettingsActiveTab = i
		}
	}
	for i, meta := range m.buildSettingsMetaForCategory(category) {
		if meta.Key == setting {
			m.SettingsSelectedRow = i
			return m
		}
	}
	t.Fatalf("setting %s/%s not found", category, setting)
	return m
}

func settingsPress(t *testing.T, m RootModel, msg tea.KeyPressMsg) RootModel {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(RootModel)
}

func settingRune(ch rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: ch, Text: string(ch)} }

func TestSelectedSettingResetRequiresConfirmation(t *testing.T) {
	m := confirmationSettingsModel(t, "Network", "max_concurrent_downloads")
	m.Settings.Network.MaxConcurrentDownloads.Value = 7
	m.Settings.General.ClipboardMonitor.Value = false
	before := m.Settings.Clone()
	m = settingsPress(t, m, settingRune('r'))
	if m.state != SettingsConfirmState || m.settingsAction.kind != settingsResetOne || !config.DiffSettings(before, m.Settings).Empty() {
		t.Fatal("r mutated settings before confirmation")
	}
	if !strings.Contains(m.View().Content, "Only this setting will be reset") {
		t.Fatal("selected-setting reset scope is missing")
	}
	m = settingsPress(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.state != SettingsState || !config.DiffSettings(before, m.Settings).Empty() {
		t.Fatal("Enter should select the default Cancel button")
	}
	m = settingsPress(t, m, settingRune('r'))
	m = settingsPress(t, m, settingRune('y'))
	if config.Resolve[int](m.Settings.Network.MaxConcurrentDownloads) != config.Resolve[int](config.DefaultSettings().Network.MaxConcurrentDownloads) || config.Resolve[bool](m.Settings.General.ClipboardMonitor) || len(m.Settings.Categories.Categories) != 2 {
		t.Fatal("r reset more than the selected setting")
	}
}

func TestShiftRResetsAllSettingsAndCategoriesAfterConfirmation(t *testing.T) {
	t.Cleanup(func() { colors.LoadTheme("", true) })
	m := confirmationSettingsModel(t, "Categories", "category_0")
	m.Settings.Network.MaxConcurrentDownloads.Value = 7
	m.Settings.General.ClipboardMonitor.Value = false
	m.keys.ApplyConfig(&config.KeyMapConfig{Dashboard: map[string]config.KeyBindingConfig{"CopyID": {Keys: []string{"ctrl+i"}}}})
	before := m.Settings.Clone()
	keysBefore := m.keys.ToConfig()
	saveCalls := 0
	m.SaveSettingsFunc = func(*config.Settings) error { saveCalls++; return nil }
	m = settingsPress(t, m, settingRune('R'))
	if m.state != SettingsConfirmState || m.settingsAction.kind != settingsResetAll || !config.DiffSettings(before, m.Settings).Empty() {
		t.Fatal("Shift+R did not request an unchanged reset-all confirmation")
	}
	if !strings.Contains(m.View().Content, "Reset all settings") || !strings.Contains(m.View().Content, "Custom keybindings are kept") {
		t.Fatal("reset-all dialog does not explain its scope")
	}
	m = settingsPress(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if !config.DiffSettings(before, m.Settings).Empty() {
		t.Fatal("cancel changed settings")
	}
	m = settingsPress(t, m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModShift, Text: "R"})
	m = settingsPress(t, m, settingRune('y'))
	if !config.DiffSettings(config.DefaultSettings(), m.Settings).Empty() {
		t.Fatal("Shift+R did not reset every setting and category rule")
	}
	if !reflect.DeepEqual(keysBefore, m.keys.ToConfig()) {
		t.Fatal("settings reset changed custom keybindings")
	}
	if saveCalls != 0 || m.SettingsBaseline == nil {
		t.Fatal("reset should remain staged until settings are saved")
	}
}

func TestDeleteCategoryUsesDAndRequiresConfirmation(t *testing.T) {
	m := confirmationSettingsModel(t, "Categories", "category_0")
	m = settingsPress(t, m, settingRune('r'))
	if m.state != SettingsState || len(m.Settings.Categories.Categories) != 2 {
		t.Fatal("r still acts on category rules")
	}
	m = settingsPress(t, m, settingRune('d'))
	if m.state != SettingsConfirmState || len(m.Settings.Categories.Categories) != 2 || !strings.Contains(m.View().Content, `Delete category "First"?`) {
		t.Fatal("d deleted without a target-specific confirmation")
	}
	m = settingsPress(t, m, settingRune('n'))
	if len(m.Settings.Categories.Categories) != 2 {
		t.Fatal("cancel deleted a category")
	}
	m = settingsPress(t, m, settingRune('d'))
	m = settingsPress(t, m, settingRune('y'))
	if m.state != SettingsState || len(m.Settings.Categories.Categories) != 1 || m.Settings.Categories.Categories[0].Name != "Second" {
		t.Fatal("confirmed deletion removed the wrong category")
	}
}

func TestCategoryShortcutsMatchContextAndConfiguredKeys(t *testing.T) {
	m := confirmationSettingsModel(t, "Categories", "category_0")
	m.keys.ApplyConfig(&config.KeyMapConfig{Settings: map[string]config.KeyBindingConfig{"DeleteCategory": {Keys: []string{"ctrl+d"}}}})
	text := stripANSI.ReplaceAllString(m.renderSettingsHelp(150), "")
	for _, hint := range []string{"ctrl+d delete category", "move up"} {
		if !strings.Contains(text, hint) {
			t.Fatalf("missing context shortcut %q: %s", hint, text)
		}
	}
	for _, wrong := range []string{"prev tab", "next tab", "reset setting", "reset all", "shift+r", "tabs", "1/2/3/4/5"} {
		if strings.Contains(text, wrong) {
			t.Fatalf("category context still advertises %q", wrong)
		}
	}
	tab, row := m.SettingsActiveTab, m.SettingsSelectedRow
	for _, msg := range []tea.KeyPressMsg{{Code: tea.KeyLeft}, {Code: tea.KeyRight}, settingRune('h'), settingRune('l')} {
		m = settingsPress(t, m, msg)
		if m.SettingsActiveTab != tab || m.SettingsSelectedRow != row {
			t.Fatal("category list unexpectedly changed tabs")
		}
	}
	m = settingsPress(t, m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.state != SettingsConfirmState {
		t.Fatal("delete category ignored configured shortcut")
	}
	m = settingsPress(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = settingsPress(t, m, settingRune('1'))
	if m.SettingsActiveTab != 0 {
		t.Fatal("numbered tabs should still leave the category list")
	}
	m = settingsPress(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.SettingsActiveTab != 1 {
		t.Fatal("normal settings lost left/right tab navigation")
	}
}

func TestManageCategoriesAllowsTabNavigation(t *testing.T) {
	for _, msg := range []tea.KeyPressMsg{{Code: tea.KeyLeft}, {Code: tea.KeyRight}, settingRune('h'), settingRune('l')} {
		m := confirmationSettingsModel(t, "Categories", "category_enabled")
		text := stripANSI.ReplaceAllString(m.renderSettingsHelp(150), "")
		if !strings.Contains(text, "prev tab") || !strings.Contains(text, "next tab") {
			t.Fatal("Manage categories should advertise tab navigation")
		}
		tab := m.SettingsActiveTab
		m = settingsPress(t, m, msg)
		if m.SettingsActiveTab == tab {
			t.Fatalf("Manage categories blocked %s tab navigation", msg.String())
		}
	}
}

func TestResetOneFromSearchTargetsOriginalCategory(t *testing.T) {
	m := confirmationSettingsModel(t, "General", "clipboard_monitor")
	m.Settings.Network.MaxConcurrentDownloads.Value = 7
	m.settingsSearchQuery = "max_concurrent_downloads"
	m.SettingsSelectedRow = 0
	m = settingsPress(t, m, settingRune('r'))
	if m.settingsAction.category != "Network" {
		t.Fatal("search reset captured the visible tab instead of the setting category")
	}
	m = settingsPress(t, m, settingRune('y'))
	if config.Resolve[int](m.Settings.Network.MaxConcurrentDownloads) != config.Resolve[int](config.DefaultSettings().Network.MaxConcurrentDownloads) {
		t.Fatal("filtered setting was not reset")
	}
}

func TestResetAllServiceFailurePreservesOtherSettings(t *testing.T) {
	m := confirmationSettingsModel(t, "Network", "max_concurrent_downloads")
	m.Settings.General.AutoStart.Value = true
	m.Settings.Network.MaxConcurrentDownloads.Value = 7
	before := m.Settings.Clone()
	m.ToggleServiceFunc = func(bool) error { return errors.New("service unavailable") }
	m = settingsPress(t, m, settingRune('R'))
	m = settingsPress(t, m, settingRune('y'))
	if !config.DiffSettings(before, m.Settings).Empty() || !strings.Contains(m.settingsError, "service unavailable") {
		t.Fatal("failed service transition partially reset settings")
	}
}

func TestReadOnlySettingsCannotResetOrDelete(t *testing.T) {
	for _, ch := range []rune{'r', 'R', 'd'} {
		m := confirmationSettingsModel(t, "Categories", "category_0")
		m.SettingsReadOnly = true
		m = settingsPress(t, m, settingRune(ch))
		if m.state != SettingsState || m.settingsAction != nil || m.settingsError == "" || len(m.Settings.Categories.Categories) != 2 {
			t.Fatalf("read-only action %c was not blocked", ch)
		}
	}
}
