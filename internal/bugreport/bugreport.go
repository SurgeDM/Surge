package bugreport

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/SurgeDM/Surge/internal/config"
)

const newIssueURL = "https://github.com/SurgeDM/Surge/issues/new"

const extensionTemplate = "extension_bug_report.md"

// CoreReportOptions controls optional prefilled sections for core bug reports.
type CoreReportOptions struct {
	Version              string
	Commit               string
	IncludeSystemDetails bool
	IncludeLatestLogPath bool
}

// CoreBugReportURL builds a core bug report URL using body prefill only.
func CoreBugReportURL(options CoreReportOptions) string {
	issueURL, err := url.Parse(newIssueURL)
	if err != nil {
		return ""
	}

	params := url.Values{}
	params.Set("title", "Bug: ")
	params.Set("body", coreIssueBody(options))
	issueURL.RawQuery = params.Encode()

	return issueURL.String()
}

// ExtensionBugReportURL builds an extension bug report URL using template-only mode.
func ExtensionBugReportURL() string {
	issueURL, err := url.Parse(newIssueURL)
	if err != nil {
		return ""
	}

	params := url.Values{}
	params.Set("template", extensionTemplate)
	issueURL.RawQuery = params.Encode()

	return issueURL.String()
}

// LatestDebugLogPath returns the newest debug-*.log path under Surge logs dir.
func LatestDebugLogPath() (string, bool) {
	logsDir := config.GetLogsDir()
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return "", false
	}

	latest := ""
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "debug-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		if name > latest {
			latest = name
		}
	}

	if latest == "" {
		return "", false
	}

	return filepath.Join(logsDir, latest), true
}

func coreIssueBody(options CoreReportOptions) string {
	var b strings.Builder

	b.WriteString("**Describe the bug**\n")
	b.WriteString("A clear and concise description of what the bug is.\n\n")

	b.WriteString("**To Reproduce**\n")
	b.WriteString("Steps to reproduce the behavior:\n\n")
	b.WriteString("1. Go to '...'\n")
	b.WriteString("2. Press '....'\n")
	b.WriteString("3. Scroll down to '....'\n")
	b.WriteString("4. See error/unexpected behaviour\n\n")

	b.WriteString("**Expected behavior**\n")
	b.WriteString("A clear and concise description of what you expected to happen.\n\n")

	b.WriteString("**Screenshots**\n")
	b.WriteString("If applicable, add screenshots to help explain your problem.\n\n")

	b.WriteString("**Logs**\n")
	b.WriteString("Surge automatically writes debug log files.\n\n")
	b.WriteString("1. The log file is written to:\n")
	b.WriteString("   - **Linux:** `~/.local/state/surge/logs/`\n")
	b.WriteString("   - **macOS:** `~/Library/Application Support/surge/logs/`\n")
	b.WriteString("   - **Windows:** `%APPDATA%\\surge\\logs\\`\n")
	b.WriteString("2. Attach the most recent `debug-*.log` file by dragging it into this issue, or paste relevant excerpts in a code block.\n")

	if options.IncludeLatestLogPath {
		if latestLogPath, ok := LatestDebugLogPath(); ok {
			b.WriteString("\nYour latest log: ")
			b.WriteString(latestLogPath)
			b.WriteString("\nPlease drag-attach this file once the issue page opens.\n")
		} else {
			b.WriteString("\nYour latest log could not be auto-detected. Please attach the newest `debug-*.log` file manually once the issue page opens.\n")
		}
	}

	b.WriteString("\n**Please complete the following information:**\n\n")

	if options.IncludeSystemDetails {
		b.WriteString("- OS: ")
		b.WriteString(runtime.GOOS)
		b.WriteString("/")
		b.WriteString(runtime.GOARCH)
		b.WriteString("\n")
		b.WriteString("- Surge Version: ")
		b.WriteString(normalizeValue(options.Version))
		b.WriteString("\n")
		b.WriteString("- Commit: ")
		b.WriteString(normalizeValue(options.Commit))
		b.WriteString("\n")
	} else {
		b.WriteString("- OS: [e.g. Windows 11 / macOS 14 / Ubuntu 24.04]\n")
		b.WriteString("- Surge Version: [e.g. 1.2.0 - run surge --version]\n")
		b.WriteString("- Commit: [e.g. 9f3d2ab]\n")
	}
	b.WriteString("- Installed From: [e.g. Brew / GitHub Release / built from source]\n\n")

	b.WriteString("**Additional context**\n")
	b.WriteString("Add any other context about the problem here.\n")

	return b.String()
}

func normalizeValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}
