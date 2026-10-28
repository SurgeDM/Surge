package tui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/types"
)

func allTabTestModel() RootModel {
	InitializeTUI()
	return RootModel{
		Settings: config.DefaultSettings(), keys: config.DefaultKeyMap(), pinnedTab: -1,
		list: NewDownloadList(100, 30), logViewport: viewport.New(viewport.WithWidth(80), viewport.WithHeight(5)),
		downloads: []*DownloadModel{
			{ID: "active", Filename: "report.pdf", FilenameLower: "report.pdf", started: true},
			{ID: "queued", Filename: "archive.zip", FilenameLower: "archive.zip"},
			{ID: "paused", paused: true},
			{ID: "pausing", pausing: true},
			{ID: "resuming", started: true, resuming: true},
			{ID: "completed", done: true},
			{ID: "failed", done: true, err: errors.New("failed")},
		},
	}
}

func TestAllTabIncludesEveryStateAndRespectsFilters(t *testing.T) {
	m := allTabTestModel()
	updated, _ := m.updateDashboard(tea.KeyPressMsg{Code: '0', Text: "0"})
	m = updated.(RootModel)
	if m.activeTab != TabAll || len(m.list.Items()) != 7 {
		t.Fatal("All shortcut did not include all seven download states")
	}
	stats := m.ComputeViewStats()
	if !strings.Contains(stripANSI.ReplaceAllString(m.renderTabs(TabAll, stats.ActiveCount, stats.QueuedCount, stats.DownloadedCount), ""), "All (7)") {
		t.Fatal("All tab count does not include every download")
	}
	m.searchQuery = "report"
	m.UpdateListItems()
	if len(m.list.Items()) != 1 || m.GetSelectedDownload().ID != "active" {
		t.Fatal("All tab ignored the search query")
	}
	m.searchQuery = ""
	m.Settings.Categories.CategoryEnabled.Value = true
	m.categoryFilter = "Documents"
	m.UpdateListItems()
	if len(m.list.Items()) != 1 || m.GetSelectedDownload().ID != "active" {
		t.Fatal("All tab ignored the category filter")
	}
	m.SelectedDownloadID = "queued" // Excluded by the current category filter.
	m.UpdateListItems()
	if m.activeTab != TabAll {
		t.Fatal("auto-follow left All to reveal an excluded download")
	}
}

func TestAllTabNavigationAndPinning(t *testing.T) {
	m := allTabTestModel()
	m.activeTab = TabAll
	press := func(msg tea.KeyPressMsg) { t.Helper(); updated, _ := m.updateDashboard(msg); m = updated.(RootModel) }
	for _, want := range []int{TabQueued, TabActive, TabDone, TabAll} {
		press(tea.KeyPressMsg{Code: tea.KeyTab})
		if m.activeTab != want {
			t.Fatalf("next tab = %d, want %d", m.activeTab, want)
		}
	}
	for _, want := range []int{TabDone, TabActive, TabQueued, TabAll} {
		press(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		if m.activeTab != want {
			t.Fatalf("previous tab = %d, want %d", m.activeTab, want)
		}
	}
	press(tea.KeyPressMsg{Code: 't', Text: "t"})
	if m.pinnedTab != TabAll || !strings.Contains(strings.Join(m.logEntries, "\n"), "Pinned: All") {
		t.Fatal("All tab did not pin correctly")
	}
	press(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.activeTab != TabAll {
		t.Fatal("tab navigation escaped pinned All tab")
	}
	press(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.keys.ApplyConfig(&config.KeyMapConfig{Dashboard: map[string]config.KeyBindingConfig{"TabAll": {Keys: []string{"ctrl+a"}}}})
	press(tea.KeyPressMsg{Code: 'q', Text: "q"})
	press(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if m.activeTab != TabAll {
		t.Fatal("All shortcut ignored configured keybinding")
	}
}

func TestAllTabStaysSelectedAsDownloadsStartAndComplete(t *testing.T) {
	m := allTabTestModel()
	m.activeTab = TabAll
	m.UpdateListItems()
	updated, _ := m.handleDownloadEvent(types.DownloadEvent{Type: types.EventStarted, DownloadID: "new", Filename: "new.zip", Total: 100})
	m = updated.(RootModel)
	if m.activeTab != TabAll || len(m.list.Items()) != 8 {
		t.Fatal("starting a download left All or hid the new download")
	}
	updated, _ = m.handleDownloadEvent(types.DownloadEvent{Type: types.EventComplete, DownloadID: "new", Total: 100})
	m = updated.(RootModel)
	if m.activeTab != TabAll || len(m.list.Items()) != 8 {
		t.Fatal("completing a download left All or hid the completed download")
	}
}
