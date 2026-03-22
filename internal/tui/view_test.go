package tui

import (
	"github.com/surge-downloader/surge/internal/processing"
	"regexp"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/surge-downloader/surge/internal/tui/colors"
)

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestFormatDurationForUI(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		want string
	}{
		{"zero", 0, "0:00"},
		{"negative", -5 * time.Second, "0:00"},
		{"30 seconds", 30 * time.Second, "0:30"},
		{"1 minute", 60 * time.Second, "1:00"},
		{"5m30s", 5*time.Minute + 30*time.Second, "5:30"},
		{"59m59s", 59*time.Minute + 59*time.Second, "59:59"},
		{"1 hour", time.Hour, "1:00:00"},
		{"1h2m3s", time.Hour + 2*time.Minute + 3*time.Second, "1:02:03"},
		{"23h59m59s", 23*time.Hour + 59*time.Minute + 59*time.Second, "23:59:59"},
		{"1 day", 24 * time.Hour, "1d"},
		{"1d 5h", 29 * time.Hour, "1d 5h"},
		{"30+ days", 31 * 24 * time.Hour, "∞"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDurationForUI(tt.dur)
			if got != tt.want {
				t.Errorf("formatDurationForUI(%v) = %q, want %q", tt.dur, got, tt.want)
			}
		})
	}
}

func TestGetDownloadStatus(t *testing.T) {
	spinnerView := "⠋"

	tests := []struct {
		name     string
		model    *DownloadModel
		expected string
	}{
		{
			name: "Pausing State",
			model: &DownloadModel{
				pausing: true,
			},
			expected: "⠋ Pausing...",
		},
		{
			name: "Resuming State",
			model: &DownloadModel{
				resuming: true,
			},
			expected: "⠋ Resuming...",
		},
		{
			name: "Queued State",
			model: &DownloadModel{
				Speed:      0,
				Downloaded: 0,
				done:       false,
				paused:     false,
				err:        nil,
			},
			expected: "⠋ Queued",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := getDownloadStatus(tt.model, spinnerView)
			plainStatus := ansiEscapeRE.ReplaceAllString(status, "")
			if !strings.Contains(plainStatus, tt.expected) {
				t.Errorf("getDownloadStatus() = %q, want it to contain %q", plainStatus, tt.expected)
			}
		})
	}
}

func TestView_DashboardFitsViewportWithoutTopCutoff(t *testing.T) {
	m := InitialRootModel(1701, "test-version", nil, processing.NewLifecycleManager(nil, nil), false)

	cases := []struct {
		width  int
		height int
	}{
		{120, 35},
		{100, 30},
		{80, 24},
	}

	for _, tc := range cases {
		m.width = tc.width
		m.height = tc.height

		view := m.View()
		if strings.HasPrefix(view.Content, "\n") {
			t.Fatalf("view starts with a blank line at %dx%d", tc.width, tc.height)
		}

		plain := ansiEscapeRE.ReplaceAllString(view.Content, "")
		trimmed := strings.TrimRight(plain, "\n")
		lines := strings.Split(trimmed, "\n")

		if len(lines) > tc.height {
			t.Fatalf("view exceeds viewport height at %dx%d: got %d lines", tc.width, tc.height, len(lines))
		}

		if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
			t.Fatalf("top line is empty at %dx%d (possible top cutoff)", tc.width, tc.height)
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			t.Fatalf("bottom line is empty at %dx%d (footer likely clipped)", tc.width, tc.height)
		}
	}
}

func TestView_QuitConfirmContainsButtons(t *testing.T) {
	m := InitialRootModel(1701, "test-version", nil, processing.NewLifecycleManager(nil, nil), false)
	m.state = QuitConfirmState
	m.width = 120
	m.height = 35

	plain := ansiEscapeRE.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(plain, "Yep!") {
		t.Fatal("expected Yep! button in quit confirm view")
	}
	if !strings.Contains(plain, "Nope") {
		t.Fatal("expected Nope button in quit confirm view")
	}
	if !strings.Contains(plain, "Are you sure you want to quit?") {
		t.Fatal("expected confirmation message in quit confirm view")
	}
}

func TestView_QuitConfirmShowsActiveDownloadDetail(t *testing.T) {
	m := InitialRootModel(1701, "test-version", nil, processing.NewLifecycleManager(nil, nil), false)
	m.state = QuitConfirmState
	m.width = 120
	m.height = 35
	m.downloads = []*DownloadModel{
		{Speed: 1.0}, // active download
	}

	plain := ansiEscapeRE.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(plain, "1 active download(s) will be paused") {
		t.Fatalf("expected active download detail, got:\n%s", plain)
	}
}

func TestView_QuitConfirmNoFocusedRendersCorrectly(t *testing.T) {
	m := InitialRootModel(1701, "test-version", nil, processing.NewLifecycleManager(nil, nil), false)
	m.state = QuitConfirmState
	m.quitConfirmFocused = 1
	m.width = 120
	m.height = 35

	plain := ansiEscapeRE.ReplaceAllString(m.View().Content, "")
	if !strings.Contains(plain, "Nope") {
		t.Fatal("expected Nope button present when No is focused")
	}
}

func TestView_QuitConfirmTinyTerminalDoesNotPanic(t *testing.T) {
	m := InitialRootModel(1701, "test-version", nil, processing.NewLifecycleManager(nil, nil), false)
	m.state = QuitConfirmState
	m.width = 10
	m.height = 5
	_ = m.View()
}

func TestView_SettingsTinyTerminalDoesNotPanic(t *testing.T) {
	m := InitialRootModel(1701, "test-version", nil, processing.NewLifecycleManager(nil, nil), false)
	m.state = SettingsState
	m.width = 20
	m.height = 8

	view := m.View()
	if strings.TrimSpace(ansiEscapeRE.ReplaceAllString(view.Content, "")) == "" {
		t.Fatal("expected non-empty settings view for tiny terminal")
	}
}

func TestView_NetworkActivityShowsFiveAxisLabelsWhenTall(t *testing.T) {
	m := InitialRootModel(1701, "test-version", nil, processing.NewLifecycleManager(nil, nil), false)
	m.width = 140
	m.height = 40

	view := m.View()
	plain := ansiEscapeRE.ReplaceAllString(view.Content, "")

	if !strings.Contains(plain, "0.8 MB/s") || !strings.Contains(plain, "0.2 MB/s") {
		t.Fatalf("expected 5-axis labels (including 0.8 and 0.2 MB/s), got:\n%s", plain)
	}
}

func BenchmarkLogoGradient(b *testing.B) {
	logoText := `
   _______  ___________ ____ 
  / ___/ / / / ___/ __ '/ _ \
 (__  ) /_/ / /  / /_/ /  __/
/____/\__,_/_/   \__, /\___/ 
                /____/       `

	startColor := colors.NeonPink
	endColor := colors.NeonPurple

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ApplyGradient(logoText, startColor, endColor)
	}
}

func BenchmarkCachedLogo(b *testing.B) {
	logoText := `
   _______  ___________ ____ 
  / ___/ / / / ___/ __ '/ _ \
 (__  ) /_/ / /  / /_/ /  __/
/____/\__,_/_/   \__, /\___/ 
                /____/       `

	m := InitialRootModel(1701, "test-version", nil, processing.NewLifecycleManager(nil, nil), false)
	// Pre-warm cache
	gradientLogo := ApplyGradient(logoText, colors.NeonPink, colors.NeonPurple)
	m.logoCache = lipgloss.NewStyle().Render(gradientLogo)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if m.logoCache != "" {
			_ = m.logoCache
		} else {
			_ = ApplyGradient(logoText, colors.NeonPink, colors.NeonPurple)
		}
	}
}
