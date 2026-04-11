package bugreport

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
)

func TestCoreBugReportURL_UsesBodyOnlyAndPrefillsKnownValues(t *testing.T) {
	reportURL := CoreBugReportURL(CoreReportOptions{
		Version:              "1.2.3",
		Commit:               "abc123",
		IncludeSystemDetails: true,
	})

	parsed, err := url.Parse(reportURL)
	if err != nil {
		t.Fatalf("failed to parse core bug-report URL: %v", err)
	}

	query := parsed.Query()
	if got := query.Get("template"); got != "" {
		t.Fatalf("core URL should not include template param, got: %q", got)
	}
	if got := query.Get("title"); got != "Bug: " {
		t.Fatalf("unexpected title value: %q", got)
	}

	body := query.Get("body")
	if body == "" {
		t.Fatal("core URL should include body param")
	}
	if !strings.Contains(body, "**Describe the bug**") {
		t.Fatalf("missing describe section in body: %q", body)
	}
	if !strings.Contains(body, "**To Reproduce**") {
		t.Fatalf("missing reproduce section in body: %q", body)
	}
	if !strings.Contains(body, "**Expected behavior**") {
		t.Fatalf("missing expected behavior section in body: %q", body)
	}
	if !strings.Contains(body, "**Screenshots**") {
		t.Fatalf("missing screenshots section in body: %q", body)
	}
	if !strings.Contains(body, "**Additional context**") {
		t.Fatalf("missing additional context section in body: %q", body)
	}
	if !strings.Contains(body, "- OS: "+runtime.GOOS+"/"+runtime.GOARCH) {
		t.Fatalf("missing os/arch line in body: %q", body)
	}
	if !strings.Contains(body, "- Surge Version: 1.2.3") {
		t.Fatalf("missing version line in body: %q", body)
	}
	if !strings.Contains(body, "- Commit: abc123") {
		t.Fatalf("missing commit line in body: %q", body)
	}
}

func TestCoreBugReportURL_LeavesPlaceholdersWhenSystemDetailsDisabled(t *testing.T) {
	reportURL := CoreBugReportURL(CoreReportOptions{
		Version:              "1.2.3",
		Commit:               "abc123",
		IncludeSystemDetails: false,
	})

	parsed, err := url.Parse(reportURL)
	if err != nil {
		t.Fatalf("failed to parse core bug-report URL: %v", err)
	}
	body := parsed.Query().Get("body")

	if !strings.Contains(body, "- OS: [e.g. Windows 11 / macOS 14 / Ubuntu 24.04]") {
		t.Fatalf("expected OS placeholder line in body: %q", body)
	}
	if !strings.Contains(body, "- Surge Version: [e.g. 1.2.0 - run surge --version]") {
		t.Fatalf("expected version placeholder line in body: %q", body)
	}
	if !strings.Contains(body, "- Commit: [e.g. 9f3d2ab]") {
		t.Fatalf("expected commit placeholder line in body: %q", body)
	}
}

func TestCoreBugReportURLNormalizesEmptyInputs(t *testing.T) {
	tests := []struct {
		version string
		commit  string
	}{
		{"", ""},
		{"  ", "  "},
	}

	for _, tc := range tests {
		reportURL := CoreBugReportURL(CoreReportOptions{
			Version:              tc.version,
			Commit:               tc.commit,
			IncludeSystemDetails: true,
		})
		parsed, err := url.Parse(reportURL)
		if err != nil {
			t.Fatalf("failed to parse core bug-report URL: %v", err)
		}

		body := parsed.Query().Get("body")
		if !strings.Contains(body, "- Surge Version: unknown") {
			t.Errorf("expected unknown version fallback, got: %q", body)
		}
		if !strings.Contains(body, "- Commit: unknown") {
			t.Errorf("expected unknown commit fallback, got: %q", body)
		}
	}
}

func TestCoreBugReportURLIncludesLatestLogPathWhenRequested(t *testing.T) {
	logsDir := prepareTempLogsDir(t)
	latest := filepath.Join(logsDir, "debug-20260413-120000.log")
	if err := os.WriteFile(latest, []byte("latest"), 0o644); err != nil {
		t.Fatalf("failed to create latest log file: %v", err)
	}

	reportURL := CoreBugReportURL(CoreReportOptions{
		Version:              "1.2.3",
		Commit:               "abc123",
		IncludeSystemDetails: true,
		IncludeLatestLogPath: true,
	})

	parsed, err := url.Parse(reportURL)
	if err != nil {
		t.Fatalf("failed to parse core bug-report URL: %v", err)
	}
	body := parsed.Query().Get("body")
	if !strings.Contains(body, "Your latest log: "+latest) {
		t.Fatalf("expected latest log path note in body: %q", body)
	}
	if !strings.Contains(body, "Please drag-attach this file once the issue page opens.") {
		t.Fatalf("expected drag-attach instruction in body: %q", body)
	}
}

func TestExtensionBugReportURL_UsesTemplateOnly(t *testing.T) {
	reportURL := ExtensionBugReportURL()
	parsed, err := url.Parse(reportURL)
	if err != nil {
		t.Fatalf("failed to parse extension bug-report URL: %v", err)
	}

	query := parsed.Query()
	if got := query.Get("template"); got != "extension_bug_report.md" {
		t.Fatalf("unexpected template value: %q", got)
	}
	if got := query.Get("body"); got != "" {
		t.Fatalf("extension URL should not include body param, got: %q", got)
	}
}

func TestLatestDebugLogPath_NoLogs(t *testing.T) {
	_ = prepareTempLogsDir(t)

	logPath, ok := LatestDebugLogPath()
	if ok {
		t.Fatalf("expected no log path, got: %q", logPath)
	}
}

func TestLatestDebugLogPath_SelectsNewestDebugLog(t *testing.T) {
	logsDir := prepareTempLogsDir(t)

	older := filepath.Join(logsDir, "debug-20260412-120000.log")
	newer := filepath.Join(logsDir, "debug-20260413-120000.log")
	ignored := filepath.Join(logsDir, "notes.log")

	for _, item := range []string{older, newer, ignored} {
		if err := os.WriteFile(item, []byte("x"), 0o644); err != nil {
			t.Fatalf("failed to create log fixture %q: %v", item, err)
		}
	}

	logPath, ok := LatestDebugLogPath()
	if !ok {
		t.Fatal("expected to find latest debug log path")
	}
	if logPath != newer {
		t.Fatalf("latest debug log = %q, want %q", logPath, newer)
	}
}

func prepareTempLogsDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", root)
	} else {
		t.Setenv("XDG_STATE_HOME", root)
	}

	logsDir := config.GetLogsDir()
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("failed to create logs directory: %v", err)
	}
	return logsDir
}
