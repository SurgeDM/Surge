package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

func TestHelpSearchMatchesDownloadSearchInteraction(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, nil, nil, false)
	m.helpSearchInput = textinput.New()
	m.state = HelpModalState
	m.width, m.height = 120, 40
	m.searchQuery = "download filter"
	press := func(code rune) {
		t.Helper()
		msg := tea.KeyPressMsg{Code: code}
		if code >= 32 && code < 127 {
			msg.Text = string(code)
		}
		updated, _ := m.Update(msg)
		m = updated.(RootModel)
	}
	press('/')
	for _, ch := range "pause" {
		press(ch)
	}
	if !m.helpSearchActive || m.helpSearchQuery != "pause" {
		t.Fatalf("search = %q, active = %v", m.helpSearchQuery, m.helpSearchActive)
	}
	text := stripANSI.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(text, "pause") || strings.Contains(text, "add download") {
		t.Fatalf("help was not filtered: %s", text)
	}
	press(tea.KeyEnter)
	if m.helpSearchActive || m.helpSearchQuery != "pause" {
		t.Fatal("Enter should keep the filter")
	}
	press('/')
	press('h')
	if m.state != HelpModalState || m.helpSearchQuery != "pauseh" {
		t.Fatal("shortcut key escaped search input")
	}
	press(tea.KeyEsc)
	if m.state != HelpModalState || m.helpSearchQuery != "" || m.helpSearchActive {
		t.Fatal("Esc should clear search and keep help open")
	}
	if m.searchQuery != "download filter" {
		t.Fatal("help search changed download search")
	}
	press(tea.KeyEsc)
	if m.state != DashboardState {
		t.Fatal("Esc should close help outside search")
	}
}

func TestHelpSearchPasteAndNoMatches(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, nil, nil, false)
	m.state = HelpModalState
	m.width, m.height = 120, 40
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(RootModel)
	updated, _ = m.Update(tea.PasteMsg{Content: "no-such-shortcut"})
	m = updated.(RootModel)
	if m.helpSearchQuery != "no-such-shortcut" {
		t.Fatalf("pasted search = %q", m.helpSearchQuery)
	}
	if !strings.Contains(m.View().Content, "No matching shortcuts") {
		t.Fatal("missing empty search message")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'Q', Text: "Q"})
	m = updated.(RootModel)
	if m.helpSearchActive || m.helpSearchQuery != "" || m.state != HelpModalState {
		t.Fatal("Q should clear help search")
	}
}

func TestCtrlCClearsSearchWithoutClosingView(t *testing.T) {
	for _, state := range []UIState{DashboardState, HelpModalState} {
		m := InitialRootModel(1701, "test", nil, nil, nil, false)
		m.state = state
		updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
		m = updated.(RootModel)
		updated, _ = m.Update(tea.PasteMsg{Content: "pause"})
		m = updated.(RootModel)
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		m = updated.(RootModel)
		if m.state != state || m.searchActive || m.helpSearchActive || m.searchQuery != "" || m.helpSearchQuery != "" {
			t.Fatalf("Ctrl+C did not clear search in state %v", state)
		}
	}
}

func TestHelpSearchHonorsRemappedSlashHelp(t *testing.T) {
	m := InitialRootModel(1701, "test", nil, nil, nil, false)
	m.state = HelpModalState
	m.keys.Dashboard.Search = key.NewBinding(key.WithKeys("s"))
	m.keys.Dashboard.ToggleHelp = key.NewBinding(key.WithKeys("/"))
	updated, _ := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = updated.(RootModel)
	if m.state != DashboardState || m.helpSearchActive {
		t.Fatal("slash did not honor remapped help toggle")
	}
	m.state = HelpModalState
	updated, _ = m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(RootModel)
	if !m.helpSearchActive {
		t.Fatal("configured search key did not activate search")
	}
}
