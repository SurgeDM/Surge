package colors

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func colorHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
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
	if got := colorHex(Pink()); got != "#d10074" {
		t.Fatalf("light Pink = %q, want #d10074", got)
	}
	if got := colorHex(StateDownloading()); got != "#2e7d32" {
		t.Fatalf("light StateDownloading = %q, want #2e7d32", got)
	}

	SetDarkMode(true)
	if got := colorHex(Pink()); got != "#ff79c6" {
		t.Fatalf("dark Pink = %q, want #ff79c6", got)
	}
	if got := colorHex(StateDownloading()); got != "#50fa7b" {
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
				_ = Pink()
				_ = StateDone()
			}
		}()
	}

	wg.Wait()
}

func TestLoadTheme_SingleScheme(t *testing.T) {
	prevPath := lastThemePath
	prevMode := IsDarkMode()
	t.Cleanup(func() { LoadTheme(prevPath, prevMode) })

	tmpDir := t.TempDir()
	themePath := filepath.Join(tmpDir, "test-theme.toml")

	content := `
[colors]
name = "Test Theme"
[colors.primary]
background = "#123456"
foreground = "#654321"

[colors.normal]
black = "#000001"
red = "#000002"
green = "#000003"
yellow = "#000004"
blue = "#000005"
magenta = "#000006"
cyan = "#000007"
white = "#000008"

[colors.bright]
black = "#000009"
red = "#00000a"
green = "#00000b"
yellow = "#00000c"
blue = "#00000d"
magenta = "#00000e"
cyan = "#00000f"
white = "#000010"
`
	if err := os.WriteFile(themePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test theme: %v", err)
	}

	LoadTheme(themePath, true)

	if got := colorHex(Background()); got != "#123456" {
		t.Errorf("Background() = %q, want #123456", got)
	}
	if got := colorHex(Pink()); got != "#00000a" {
		t.Errorf("Pink() = %q, want #00000a (bright red)", got)
	}
}
