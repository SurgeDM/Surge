package colors

import (
	"fmt"
	"image/color"
	"sync"
	"testing"
)

func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8) /*nolint:gosec*/, uint8(g>>8), uint8(b>>8))
}

func TestThemeColor_RespectsDarkMode(t *testing.T) {
	prev := IsDarkMode()
	t.Cleanup(func() { SetDarkMode(prev) })

	SetDarkMode(false)
	if got := colorHex(ThemeColor("#111111", "#222222")); got != "#111111" {
		t.Fatalf("light mode theme color = %q, want #111111", got)
	}

	SetDarkMode(true)
	if got := colorHex(ThemeColor("#111111", "#222222")); got != "#222222" {
		t.Fatalf("dark mode theme color = %q, want #222222", got)
	}
}

func TestSetDarkMode_UpdatesExportedPalette(t *testing.T) {
	prev := IsDarkMode()
	t.Cleanup(func() { SetDarkMode(prev) })

	SetDarkMode(false)
	if got := colorHex(NeonPink); got != "#d10074" {
		t.Fatalf("light NeonPink = %q, want #d10074", got)
	}
	if got := colorHex(StateDownloading); got != "#2e7d32" {
		t.Fatalf("light StateDownloading = %q, want #2e7d32", got)
	}

	SetDarkMode(true)
	if got := colorHex(NeonPink); got != "#ff79c6" {
		t.Fatalf("dark NeonPink = %q, want #ff79c6", got)
	}
	if got := colorHex(StateDownloading); got != "#50fa7b" {
		t.Fatalf("dark StateDownloading = %q, want #50fa7b", got)
	}
}

func TestSetDarkMode_ConcurrentAccess(t *testing.T) {
	prev := IsDarkMode()
	t.Cleanup(func() { SetDarkMode(prev) })

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			SetDarkMode(i%2 == 0)
		}
	}()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = ThemeColor("#010101", "#fefefe")
				_ = IsDarkMode()
				_ = NeonPink
				_ = StateDone
			}
		}()
	}

	wg.Wait()
}
