package tui

import (
	"github.com/SurgeDM/Surge/internal/tui/components"
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestPostDownloadStatusAndDetails(t *testing.T) {
	components.InitializeStatusCache()
	d := &DownloadModel{done: true, Filename: "show.zip", processing: true}
	if got := ansi.Strip(getDownloadStatus(d, "*")); !strings.Contains(got, "Running post-download hooks") {
		t.Fatal(got)
	}
	d.processing, d.processed = false, true
	if got := ansi.Strip(getDownloadStatus(d, "*")); !strings.Contains(got, "Completed") {
		t.Fatal(got)
	}
	for _, errorText := range []string{"", "Post-download command failed: exit status 2"} {
		d.hookError = errorText
		if got := ansi.Strip(getDownloadStatus(d, "*")); !strings.Contains(got, "Completed") {
			t.Fatal(got)
		}
		details := ansi.Strip(renderFocusedDetails(d, 90, "*"))
		expected := "Done"
		if errorText != "" {
			expected = "Failed (exit status 2)"
		}
		if !strings.Contains(details, "Processing:") || !strings.Contains(details, expected) {
			t.Fatal(details)
		}
	}
}
