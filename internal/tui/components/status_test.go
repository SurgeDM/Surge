package components

import (
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/tui/colors"
)

func TestStatusRender_ReflectsThemeChanges(t *testing.T) {
	prev := colors.IsDarkMode()
	t.Cleanup(func() { colors.SetDarkMode(prev) })

	colors.SetDarkMode(false)
	light := StatusDownloading.Render()

	colors.SetDarkMode(true)
	dark := StatusDownloading.Render()

	if light == dark {
		t.Fatal("expected status rendering to change when theme changes")
	}
}

func TestStatusRenderWithSpinner(t *testing.T) {
	spinnerFrame := "⠋"

	queuedStr := StatusQueued.RenderWithSpinner(spinnerFrame)
	if !strings.Contains(queuedStr, spinnerFrame+" Queued") {
		t.Errorf("expected Queued status to contain '%s Queued', got: %s", spinnerFrame, queuedStr)
	}

	downloadingStr := StatusDownloading.RenderWithSpinner(spinnerFrame)
	if strings.Contains(downloadingStr, spinnerFrame) {
		t.Errorf("expected Downloading status to ignore spinner '%s', got: %s", spinnerFrame, downloadingStr)
	}
}
