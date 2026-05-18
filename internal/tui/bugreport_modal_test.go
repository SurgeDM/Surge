package tui

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func TestUpdateDashboard_ReportBugEntersTargetModal(t *testing.T) {
	m := newBugReportModalTestModel()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	m2 := updated.(RootModel)
	if m2.state != BugReportTargetState {
		t.Fatalf("state = %v, want %v", m2.state, BugReportTargetState)
	}
}

func TestUpdate_BugReportTargetCoreTransitionsToSystemDetails(t *testing.T) {
	m := newBugReportModalTestModel()
	m.state = BugReportTargetState

	updated, _ := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m2 := updated.(RootModel)
	if m2.state != BugReportSystemDetailsState {
		t.Fatalf("state = %v, want %v", m2.state, BugReportSystemDetailsState)
	}
}

func TestUpdate_BugReportTargetExtensionImmediatelyOpensTemplateURL(t *testing.T) {
	m := newBugReportModalTestModel()
	m.state = BugReportTargetState

	openedURL := ""
	origOpen := openBugReportBrowser
	openBugReportBrowser = func(rawURL string) error {
		openedURL = rawURL
		return nil
	}
	defer func() { openBugReportBrowser = origOpen }()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m2 := updated.(RootModel)
	if m2.state != DashboardState {
		t.Fatalf("state = %v, want %v", m2.state, DashboardState)
	}

	parsed, err := url.Parse(openedURL)
	if err != nil {
		t.Fatalf("failed to parse opened URL: %v", err)
	}
	query := parsed.Query()
	if got := query.Get("template"); got != "extension_bug_report.md" {
		t.Fatalf("template = %q, want extension_bug_report.md", got)
	}
	if got := query.Get("body"); got != "" {
		t.Fatalf("body should be empty for extension report, got %q", got)
	}
	if len(m2.logEntries) == 0 || !strings.Contains(m2.logEntries[len(m2.logEntries)-1], "Opening browser to file bug report...") {
		t.Fatalf("expected success open message in latest log entry, got: %v", m2.logEntries)
	}
}

func TestUpdate_BugReportCoreFlow_SystemDetailsNoAndLogNo_ImmediatelyOpens(t *testing.T) {
	m := newBugReportModalTestModel()
	m.state = BugReportTargetState

	openedURL := ""
	origOpen := openBugReportBrowser
	openBugReportBrowser = func(rawURL string) error {
		openedURL = rawURL
		return nil
	}
	defer func() { openBugReportBrowser = origOpen }()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	m = updated.(RootModel)
	if m.state != BugReportSystemDetailsState {
		t.Fatalf("state = %v, want %v", m.state, BugReportSystemDetailsState)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(RootModel)
	if m.state != BugReportLogPathState {
		t.Fatalf("state = %v, want %v", m.state, BugReportLogPathState)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m2 := updated.(RootModel)
	if m2.state != DashboardState {
		t.Fatalf("state = %v, want %v", m2.state, DashboardState)
	}

	parsed, err := url.Parse(openedURL)
	if err != nil {
		t.Fatalf("failed to parse opened URL: %v", err)
	}
	body := parsed.Query().Get("body")
	if !strings.Contains(body, "- OS: [e.g. Windows 11 / macOS 14 / Ubuntu 24.04]") {
		t.Fatalf("missing OS placeholder in body: %q", body)
	}
	if !strings.Contains(body, "- Surge Version: [e.g. 1.2.0 - run surge --version]") {
		t.Fatalf("missing version placeholder in body: %q", body)
	}
	if strings.Contains(body, "Your latest log") || strings.Contains(body, "auto-detected") {
		t.Fatalf("latest log note should be omitted when user chooses no: %q", body)
	}
	if len(m2.logEntries) == 0 || !strings.Contains(m2.logEntries[len(m2.logEntries)-1], "Opening browser to file bug report...") {
		t.Fatalf("expected success open message in latest log entry, got: %v", m2.logEntries)
	}
}

func TestUpdate_BugReportOpenFailureCopiesURLToClipboardAndLogsHint(t *testing.T) {
	m := newBugReportModalTestModel()
	m.state = BugReportTargetState

	origOpen := openBugReportBrowser
	openBugReportBrowser = func(rawURL string) error {
		return errors.New("open failed")
	}
	defer func() { openBugReportBrowser = origOpen }()

	copiedURL := ""
	origWriteClipboard := writeBugReportClipboard
	writeBugReportClipboard = func(text string) error {
		copiedURL = text
		return nil
	}
	defer func() { writeBugReportClipboard = origWriteClipboard }()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m2 := updated.(RootModel)
	if m2.state != DashboardState {
		t.Fatalf("state = %v, want %v", m2.state, DashboardState)
	}
	if copiedURL == "" {
		t.Fatal("expected bug report URL to be copied to clipboard")
	}
	if !strings.Contains(copiedURL, "template=extension_bug_report.md") {
		t.Fatalf("expected extension bug report URL in clipboard, got: %q", copiedURL)
	}
	if len(m2.logEntries) == 0 {
		t.Fatal("expected at least 1 log entry")
	}
	last := m2.logEntries[len(m2.logEntries)-1]
	if !strings.Contains(last, "Could not open browser. URL copied to clipboard.") {
		t.Fatalf("expected failure message in latest log entry, got: %q", last)
	}
	for _, entry := range m2.logEntries {
		if strings.Contains(entry, "https://github.com/") {
			t.Fatalf("log should not include URL after failure, got: %q", entry)
		}
	}
}

func TestUpdate_BugReportOpenFailureClipboardWriteFailureLogsTerminalHintWithoutURL(t *testing.T) {
	m := newBugReportModalTestModel()
	m.state = BugReportTargetState

	origOpen := openBugReportBrowser
	openBugReportBrowser = func(rawURL string) error {
		return errors.New("open failed")
	}
	defer func() { openBugReportBrowser = origOpen }()

	origWriteClipboard := writeBugReportClipboard
	writeBugReportClipboard = func(text string) error {
		return errors.New("clipboard failed")
	}
	defer func() { writeBugReportClipboard = origWriteClipboard }()

	updated, _ := m.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	m2 := updated.(RootModel)
	if m2.state != DashboardState {
		t.Fatalf("state = %v, want %v", m2.state, DashboardState)
	}
	if len(m2.logEntries) == 0 {
		t.Fatal("expected at least 1 log entry")
	}
	last := m2.logEntries[len(m2.logEntries)-1]
	if !strings.Contains(last, "Could not open browser. Try running surge bug-report from your terminal instead.") {
		t.Fatalf("expected fallback failure message in latest log entry, got: %q", last)
	}
	for _, entry := range m2.logEntries {
		if strings.Contains(entry, "https://github.com/") {
			t.Fatalf("log should not include URL after failure, got: %q", entry)
		}
	}
}

func TestUpdate_BugReportModalEscapeCancels(t *testing.T) {
	tests := []UIState{
		BugReportTargetState,
		BugReportSystemDetailsState,
		BugReportLogPathState,
	}

	for _, state := range tests {
		m := newBugReportModalTestModel()
		m.state = state
		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		m2 := updated.(RootModel)
		if m2.state != DashboardState {
			t.Fatalf("state %v esc should return to dashboard, got %v", state, m2.state)
		}
	}
}

func newBugReportModalTestModel() RootModel {
	return RootModel{
		state:          DashboardState,
		keys:           Keys,
		Settings:       config.DefaultSettings(),
		list:           NewDownloadList(80, 20),
		logViewport:    viewport.New(viewport.WithWidth(40), viewport.WithHeight(5)),
		CurrentVersion: "1.2.3",
		CurrentCommit:  "abc123",
	}
}
