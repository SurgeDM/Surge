package bugreport

import (
	"net/url"
	"runtime"
	"strings"
	"testing"
)

func TestBugReportURLContainsExpectedFields(t *testing.T) {
	reportURL := BugReportURL("1.2.3", "abc123")
	if !strings.HasPrefix(reportURL, "https://github.com/SurgeDM/Surge/issues/new") {
		t.Fatalf("unexpected bug-report URL: %s", reportURL)
	}

	parsed, err := url.Parse(reportURL)
	if err != nil {
		t.Fatalf("failed to parse bug-report URL: %v", err)
	}

	query := parsed.Query()
	title, ok := query["title"]
	if !ok || len(title) == 0 {
		t.Fatalf("missing title query parameter")
	}
	if title[0] != "Bug: " {
		t.Fatalf("unexpected title value: %q", title[0])
	}

	body := query.Get("body")
	if body == "" {
		t.Fatalf("missing body query parameter")
	}

	if !strings.Contains(body, "**Describe the bug**") {
		t.Fatalf("body missing bug description section: %q", body)
	}
	if !strings.Contains(body, "**Please complete the following information:**") {
		t.Fatalf("body missing environment details section: %q", body)
	}
	if !strings.Contains(body, "- OS: "+runtime.GOOS+"/"+runtime.GOARCH) {
		t.Fatalf("body missing os/arch: %q", body)
	}
	if !strings.Contains(body, "- Surge Version: 1.2.3") {
		t.Fatalf("body missing version: %q", body)
	}
	if !strings.Contains(body, "- Commit: abc123") {
		t.Fatalf("body missing commit: %q", body)
	}
	if !strings.Contains(body, "- Installed From: [e.g. Brew / GitHub Release / built from source]") {
		t.Fatalf("body missing installed-from placeholder: %q", body)
	}
}

func TestBugReportURLRoundTripsSpecialCharacters(t *testing.T) {
	version := "v1.2.3 beta+rc"
	commit := "abc 123/+?&"

	reportURL := BugReportURL(version, commit)
	parsed, err := url.Parse(reportURL)
	if err != nil {
		t.Fatalf("failed to parse bug-report URL: %v", err)
	}

	query := parsed.Query()
	body := query.Get("body")
	if !strings.Contains(body, "- Surge Version: "+version) {
		t.Fatalf("version did not round-trip through URL encoding: %q", body)
	}
	if !strings.Contains(body, "- Commit: "+commit) {
		t.Fatalf("commit did not round-trip through URL encoding: %q", body)
	}
}

func TestBugReportURLNormalizesEmptyInputs(t *testing.T) {
	tests := []struct {
		version string
		commit  string
	}{
		{"", ""},
		{"  ", "  "},
	}

	for _, tc := range tests {
		reportURL := BugReportURL(tc.version, tc.commit)
		parsed, err := url.Parse(reportURL)
		if err != nil {
			t.Fatalf("failed to parse bug-report URL: %v", err)
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
