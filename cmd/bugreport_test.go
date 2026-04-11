package cmd

import (
	"bytes"
	"errors"
	"net/url"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunBugReportCommand_ExtensionFlowUsesTemplateURL(t *testing.T) {
	cmd, out := newBugReportTestCommand("2\n\n")

	openedURL := ""
	origOpenBrowser := openBrowser
	openBrowser = func(rawURL string) error {
		openedURL = rawURL
		return nil
	}
	defer func() { openBrowser = origOpenBrowser }()

	if err := runBugReportCommand(cmd); err != nil {
		t.Fatalf("runBugReportCommand returned error: %v", err)
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

	if !strings.Contains(out.String(), "Opening browser to file bug report...") {
		t.Fatalf("missing opening message in output: %q", out.String())
	}
}

func TestRunBugReportCommand_CoreFlowCanDisableLatestLogPath(t *testing.T) {
	cmd, _ := newBugReportTestCommand("1\ny\nn\ny\n")

	origVersion := Version
	origCommit := Commit
	Version = "1.2.3"
	Commit = "abc123"
	defer func() {
		Version = origVersion
		Commit = origCommit
	}()

	openedURL := ""
	origOpenBrowser := openBrowser
	openBrowser = func(rawURL string) error {
		openedURL = rawURL
		return nil
	}
	defer func() { openBrowser = origOpenBrowser }()

	if err := runBugReportCommand(cmd); err != nil {
		t.Fatalf("runBugReportCommand returned error: %v", err)
	}

	parsed, err := url.Parse(openedURL)
	if err != nil {
		t.Fatalf("failed to parse opened URL: %v", err)
	}
	query := parsed.Query()
	if got := query.Get("template"); got != "" {
		t.Fatalf("core report should not set template, got %q", got)
	}
	body := query.Get("body")
	if !strings.Contains(body, "- Surge Version: 1.2.3") {
		t.Fatalf("missing version line in body: %q", body)
	}
	if !strings.Contains(body, "- Commit: abc123") {
		t.Fatalf("missing commit line in body: %q", body)
	}
	if strings.Contains(body, "Your latest log") || strings.Contains(body, "auto-detected") {
		t.Fatalf("latest-log note should be absent when user answers no: %q", body)
	}
}

func TestRunBugReportCommand_CoreFlowCanDisableSystemDetails(t *testing.T) {
	cmd, _ := newBugReportTestCommand("1\nn\nn\ny\n")

	openedURL := ""
	origOpenBrowser := openBrowser
	openBrowser = func(rawURL string) error {
		openedURL = rawURL
		return nil
	}
	defer func() { openBrowser = origOpenBrowser }()

	if err := runBugReportCommand(cmd); err != nil {
		t.Fatalf("runBugReportCommand returned error: %v", err)
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
	if !strings.Contains(body, "- Commit: [e.g. 9f3d2ab]") {
		t.Fatalf("missing commit placeholder in body: %q", body)
	}
}

func TestRunBugReportCommand_CoreFlowDefaultsToIncludingLatestLogPath(t *testing.T) {
	cmd, _ := newBugReportTestCommand("\n\n\n\n")

	root := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", root)
	} else {
		t.Setenv("XDG_STATE_HOME", root)
	}

	openedURL := ""
	origOpenBrowser := openBrowser
	openBrowser = func(rawURL string) error {
		openedURL = rawURL
		return nil
	}
	defer func() { openBrowser = origOpenBrowser }()

	if err := runBugReportCommand(cmd); err != nil {
		t.Fatalf("runBugReportCommand returned error: %v", err)
	}

	parsed, err := url.Parse(openedURL)
	if err != nil {
		t.Fatalf("failed to parse opened URL: %v", err)
	}
	body := parsed.Query().Get("body")
	if !strings.Contains(body, "could not be auto-detected") {
		t.Fatalf("expected default include-log note in body, got: %q", body)
	}
}

func TestRunBugReportCommand_InvalidSelectionThenValidSelection(t *testing.T) {
	cmd, out := newBugReportTestCommand("3\n2\n\n")

	origOpenBrowser := openBrowser
	openBrowser = func(rawURL string) error { return nil }
	defer func() { openBrowser = origOpenBrowser }()

	if err := runBugReportCommand(cmd); err != nil {
		t.Fatalf("runBugReportCommand returned error: %v", err)
	}

	if !strings.Contains(out.String(), "Invalid selection. Enter 1 for Core or 2 for Extension.") {
		t.Fatalf("expected invalid selection warning in output, got: %q", out.String())
	}
}

func TestRunBugReportCommand_PrintsManualURLOnBrowserFailure(t *testing.T) {
	cmd, out := newBugReportTestCommand("2\ny\n")

	origOpenBrowser := openBrowser
	openBrowser = func(rawURL string) error {
		return errors.New("open failed")
	}
	defer func() { openBrowser = origOpenBrowser }()

	if err := runBugReportCommand(cmd); err != nil {
		t.Fatalf("runBugReportCommand should not fail when browser open fails: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Could not open browser. Please open this URL manually:\n\n") {
		t.Fatalf("missing manual URL fallback message: %q", output)
	}
	if !strings.Contains(output, "template=extension_bug_report.md") {
		t.Fatalf("missing extension report URL in output: %q", output)
	}
}

func TestRunBugReportCommand_PrintsManualURLWhenBrowserLaunchSkipped(t *testing.T) {
	cmd, out := newBugReportTestCommand("2\nn\n")

	origOpenBrowser := openBrowser
	openBrowser = func(rawURL string) error {
		t.Fatalf("openBrowser should not be called when user chooses not to open browser")
		return nil
	}
	defer func() { openBrowser = origOpenBrowser }()

	if err := runBugReportCommand(cmd); err != nil {
		t.Fatalf("runBugReportCommand returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Browser launch skipped. Please open this URL manually:\n\n") {
		t.Fatalf("missing skipped-browser fallback message: %q", output)
	}
	if !strings.Contains(output, "template=extension_bug_report.md") {
		t.Fatalf("missing extension report URL in output: %q", output)
	}
	if strings.Contains(output, "Opening browser to file bug report...") {
		t.Fatalf("opening message should not be printed when browser launch is skipped: %q", output)
	}
}

func newBugReportTestCommand(input string) (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	out := &bytes.Buffer{}
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(out)
	return cmd, out
}
