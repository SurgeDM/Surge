package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/surge-downloader/surge/internal/config"
)

func TestDashboardTabPaginatorHelpers(t *testing.T) {
	var m RootModel
	if got := m.currentDashboardTab(); got != TabQueued {
		t.Fatalf("initial dashboard tab = %d, want %d", got, TabQueued)
	}

	// Legacy mirror should seed paginator state before first access.
	mLegacy := RootModel{activeTab: TabDone}
	if got := mLegacy.currentDashboardTab(); got != TabDone {
		t.Fatalf("dashboard tab from legacy mirror = %d, want %d", got, TabDone)
	}

	m.setDashboardTab(TabDone)
	m.nextDashboardTab()
	if got := m.currentDashboardTab(); got != TabQueued {
		t.Fatalf("next tab should wrap to queued, got %d", got)
	}

	m.setDashboardTab(TabActive)
	if got := m.currentDashboardTab(); got != TabActive {
		t.Fatalf("set dashboard tab = %d, want %d", got, TabActive)
	}

	m.setDashboardTab(99)
	if got := m.currentDashboardTab(); got != TabDone {
		t.Fatalf("set out-of-range dashboard tab should clamp to done, got %d", got)
	}

	m.setDashboardTab(-8)
	if got := m.currentDashboardTab(); got != TabQueued {
		t.Fatalf("set negative dashboard tab should clamp to queued, got %d", got)
	}
}

func TestSettingsTabPaginatorHelpers(t *testing.T) {
	var m RootModel

	m.SettingsActiveTab = 3
	if got := m.currentSettingsTab(2); got != 1 {
		t.Fatalf("settings tab should clamp to last available, got %d", got)
	}

	m.nextSettingsTab(2)
	if got := m.currentSettingsTab(2); got != 0 {
		t.Fatalf("next settings tab should wrap to zero, got %d", got)
	}

	m.prevSettingsTab(2)
	if got := m.currentSettingsTab(2); got != 1 {
		t.Fatalf("prev settings tab should wrap to end, got %d", got)
	}

	m.setSettingsTab(9, 2)
	if got := m.currentSettingsTab(2); got != 1 {
		t.Fatalf("set out-of-range settings tab should clamp, got %d", got)
	}

	m.setSettingsTab(-4, 0) // zero count is normalized to one page
	if got := m.currentSettingsTab(0); got != 0 {
		t.Fatalf("set settings tab with zero categories should normalize to zero, got %d", got)
	}
}

func TestUpdateDashboardUsesPaginatorBackedTabs(t *testing.T) {
	m := RootModel{
		keys:     Keys,
		Settings: config.DefaultSettings(),
		list:     NewDownloadList(80, 20),
	}

	updated, _ := m.updateDashboard(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m2 := updated.(RootModel)
	if got := m2.currentDashboardTab(); got != TabDone {
		t.Fatalf("dashboard tab after 'e' = %d, want %d", got, TabDone)
	}

	updated, _ = m2.updateDashboard(tea.KeyPressMsg{Code: tea.KeyTab})
	m3 := updated.(RootModel)
	if got := m3.currentDashboardTab(); got != TabQueued {
		t.Fatalf("dashboard tab after tab from done should wrap to queued, got %d", got)
	}

	updated, _ = m3.updateDashboard(tea.KeyPressMsg{Code: 's', Text: "s"})
	m4 := updated.(RootModel)
	if m4.state != SettingsState {
		t.Fatalf("state after 's' = %v, want %v", m4.state, SettingsState)
	}
	if got := m4.currentSettingsTab(len(config.CategoryOrder())); got != 0 {
		t.Fatalf("settings tab after opening settings = %d, want 0", got)
	}
}

func TestUpdateSettingsUsesPaginatorBackedTabs(t *testing.T) {
	m := RootModel{
		keys:     Keys,
		Settings: config.DefaultSettings(),
		state:    SettingsState,
	}

	updated, _ := m.updateSettings(tea.KeyPressMsg{Code: '2', Text: "2"})
	m2 := updated.(RootModel)
	if got := m2.currentSettingsTab(len(config.CategoryOrder())); got != 1 {
		t.Fatalf("settings tab after '2' = %d, want 1", got)
	}

	updated, _ = m2.updateSettings(tea.KeyPressMsg{Code: tea.KeyRight})
	m3 := updated.(RootModel)
	if got := m3.currentSettingsTab(len(config.CategoryOrder())); got != 2 {
		t.Fatalf("settings tab after right = %d, want 2", got)
	}

	updated, _ = m3.updateSettings(tea.KeyPressMsg{Code: tea.KeyLeft})
	m4 := updated.(RootModel)
	if got := m4.currentSettingsTab(len(config.CategoryOrder())); got != 1 {
		t.Fatalf("settings tab after left = %d, want 1", got)
	}
}

func TestUpdateSettingsEditingAndCategoryBranches(t *testing.T) {
	m := RootModel{
		keys:              Keys,
		Settings:          config.DefaultSettings(),
		state:             SettingsState,
		SettingsIsEditing: true,
	}
	m.SettingsInput = textinput.New()
	m.SettingsInput.SetValue("true")

	updated, _ := m.updateSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := updated.(RootModel)
	if m2.SettingsIsEditing {
		t.Fatal("expected edit mode to end on enter")
	}

	// Trigger bool toggle branch (General tab row with bool setting).
	m2.SettingsSelectedRow = 2 // allow_remote_open_actions
	before := m2.Settings.General.AllowRemoteOpenActions
	updated, _ = m2.updateSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	m3 := updated.(RootModel)
	if m3.Settings.General.AllowRemoteOpenActions == before {
		t.Fatal("expected bool setting to toggle")
	}

	// Trigger categories-tab branch to open category manager.
	categories := config.CategoryOrder()
	m3.setSettingsTab(len(categories)-1, len(categories))
	updated, _ = m3.updateSettings(tea.KeyPressMsg{Code: tea.KeyEnter})
	m4 := updated.(RootModel)
	if m4.state != CategoryManagerState {
		t.Fatalf("state on categories enter = %v, want %v", m4.state, CategoryManagerState)
	}

	// Trigger reset path.
	m4.state = SettingsState
	m4.setSettingsTab(0, len(categories))
	m4.SettingsSelectedRow = 1
	updated, _ = m4.updateSettings(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m5 := updated.(RootModel)
	if m5.Settings.General.DownloadCompleteNotification != config.DefaultSettings().General.DownloadCompleteNotification {
		t.Fatal("expected reset to restore default value")
	}
}

func TestCurrentSettingsTabClampsOOBActiveTab(t *testing.T) {
	m := RootModel{SettingsActiveTab: 99}
	if got := m.currentSettingsTab(len(config.CategoryOrder())); got >= len(config.CategoryOrder()) {
		t.Fatalf("settings tab index should be clamped into range, got %d", got)
	}
}

func TestViewSettingsUsesPaginatorWhenSourcesDiverge(t *testing.T) {
	m := InitialRootModel(1701, "test-version", nil, nil, false)
	m.Settings = config.DefaultSettings()
	m.width = 120
	m.height = 34
	m.settingsTabs = newTabPaginator(len(config.CategoryOrder()))
	m.settingsTabs.Page = 0  // General
	m.SettingsActiveTab = 2 // Performance

	view := m.viewSettings()
	if view == "" {
		t.Fatal("expected non-empty settings view")
	}
	if !strings.Contains(view, "Default Download Dir") {
		t.Fatal("expected general settings to be rendered from settings paginator state")
	}
	if strings.Contains(view, "Max Task Retries") {
		t.Fatal("did not expect performance settings when paginator and mirror diverge")
	}
	if strings.Contains(view, " retries") {
		t.Fatal("did not expect performance-only unit suffix in right panel when general tab is active")
	}
}

func TestUpdateListItemsSwitchesTabViaPaginator(t *testing.T) {
	queued := NewDownloadModel("q1", "https://example.com/a.bin", "a.bin", 100)
	done := NewDownloadModel("d1", "https://example.com/b.bin", "b.bin", 100)
	done.done = true

	m := RootModel{
		downloads:          []*DownloadModel{queued, done},
		Settings:           config.DefaultSettings(),
		list:               NewDownloadList(80, 20),
		activeTab:          TabQueued,
		SelectedDownloadID: "d1",
	}

	m.UpdateListItems()
	if got := m.currentDashboardTab(); got != TabDone {
		t.Fatalf("expected list update to move to done tab, got %d", got)
	}
}
