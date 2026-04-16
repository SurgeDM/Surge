package bugreport

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"
)

const newIssueURL = "https://github.com/SurgeDM/Surge/issues/new"

// BugReportURL builds a GitHub new-issue URL pre-populated with system metadata.
// version and commit should be values injected at build time via ldflags.
func BugReportURL(version, commit string) string {
	issueURL, err := url.Parse(newIssueURL)
	if err != nil {
		return ""
	}

	body := fmt.Sprintf(`**Describe the bug**
A clear and concise description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:

1. Go to '...'
2. Press '....'
3. Scroll down to '....'
4. See error/unexpected behaviour

**Expected behavior**
A clear and concise description of what you expected to happen.

**Screenshots**
If applicable, add screenshots to help explain your problem.

**Logs**
Surge automatically writes debug log files.

1. The log file is written to:
   - **Linux:** ~/.local/state/surge/logs/
   - **macOS:** ~/Library/Application Support/surge/logs/
   - **Windows:** %%APPDATA%%\surge\logs\
2. Attach the most recent debug-*.log file by dragging it into this issue, or paste relevant excerpts in a code block.

**Please complete the following information:**

- OS: %s/%s
- Surge Version: %s
- Commit: %s
- Installed From: [e.g. Brew / GitHub Release / built from source]

**Additional context**
Add any other context about the problem here.
`, runtime.GOOS, runtime.GOARCH, normalizeValue(version), normalizeValue(commit))

	params := url.Values{}
	params.Set("title", "Bug: ")
	params.Set("body", body)
	issueURL.RawQuery = params.Encode()

	return issueURL.String()
}

func normalizeValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}
