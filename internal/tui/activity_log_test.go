package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/charmbracelet/x/ansi"
)

func TestActivityLogNavigatesMessagesAndShowsFullText(t *testing.T) {
	m := RootModel{
		width: 80, height: 14,
		Settings: config.DefaultSettings(), keys: config.DefaultKeyMap(),
		lastConfigCheckTime: time.Now(),
		logViewport:         viewport.New(viewport.WithWidth(28), viewport.WithHeight(2)),
	}
	longMessage := "Beginning " + strings.Repeat("middle text ", 100) + "last words"
	m.addLogEntry("first message")
	m.addLogEntry(longMessage)
	m.addLogEntry("latest message")

	press := func(code rune) {
		t.Helper()
		updated, _ := m.Update(tea.KeyPressMsg{Code: code})
		m = updated.(RootModel)
	}

	press('l')
	if !m.logFocused || m.logCursor != 2 {
		t.Fatalf("expected focus on newest message, got focused=%v cursor=%d", m.logFocused, m.logCursor)
	}
	press(tea.KeyUp)
	if m.logCursor != 1 || m.logViewport.YOffset() != 1 {
		t.Fatalf("expected the two-line message to be selected and fully visible, got cursor=%d offset=%d", m.logCursor, m.logViewport.YOffset())
	}
	press(tea.KeyUp)
	if m.logCursor != 0 || m.logViewport.YOffset() != 0 {
		t.Fatalf("expected one step to reach the first message, got cursor=%d offset=%d", m.logCursor, m.logViewport.YOffset())
	}
	press(tea.KeyDown)
	m.addLogEntry("another message")
	press(tea.KeyEnter)
	if m.state != LogMessageState || !strings.HasSuffix(m.logMessage, longMessage) {
		t.Fatal("expected Enter to open the selected message, even after another entry arrived")
	}
	content := strings.ReplaceAll(m.logMessageViewport.GetContent(), "\n", "")
	if content != m.logMessage {
		t.Fatal("expected the modal to retain the full message after wrapping")
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Beginning") {
		t.Fatal("expected the modal to show the beginning")
	}
	press(tea.KeyDown)
	if m.logMessageViewport.YOffset() != 1 {
		t.Fatal("expected the long modal content to scroll")
	}
	m.logMessageViewport.GotoBottom()
	if !strings.Contains(ansi.Strip(m.View().Content), "last words") {
		t.Fatal("expected the modal to show the end after scrolling")
	}
	press(tea.KeyEscape)
	if m.state != DashboardState || !m.logFocused || m.logCursor != 1 {
		t.Fatal("expected Escape to return to the selected log message")
	}
}
