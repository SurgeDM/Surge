package components

import (
	"testing"

	"github.com/surge-downloader/surge/internal/tui/colors"
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
