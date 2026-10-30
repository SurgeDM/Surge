package tui

import (
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
)

func TestStripBackgroundColors(t *testing.T) {
	input := "\x1b[1;38;2;10;20;30;48;2;40;50;60mtext\x1b[49m\x1b[44;4mnext\x1b[0m"
	want := "\x1b[1;38;2;10;20;30mtext\x1b[4mnext\x1b[0m"
	if got := stripBackgroundColors(input); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTransparentBackgroundViews(t *testing.T) {
	InitializeTUI()
	settings := config.DefaultSettings()
	m := InitialRootModel(1701, "test", nil, nil, settings, false)
	m.width, m.height = 160, 48
	m.Settings.General.TransparentBackground.Value = true
	for state := DashboardState; state <= RemoveConfirmState; state++ {
		m.state = state
		view := m.View()
		if view.BackgroundColor != nil {
			t.Fatalf("state %d has a background color", state)
		}
		if view.Content == "" || stripBackgroundColors(view.Content) != view.Content {
			t.Fatalf("state %d has empty content or painted background", state)
		}
	}
	m.Settings.General.TransparentBackground.Value = false
	if view := m.View(); view.BackgroundColor == nil || !strings.Contains(view.Content, "\x1b[") {
		t.Fatal("disabling transparency did not restore themed rendering")
	}
}

func TestTransparentConfirmationHighlightsFocusedButton(t *testing.T) {
	InitializeTUI()
	m := InitialRootModel(1701, "test", nil, nil, config.DefaultSettings(), false)
	m.width, m.height, m.state = 100, 30, QuitConfirmState
	m.Settings.General.TransparentBackground.Value = true
	for _, tc := range []struct {
		focus           int
		selected, other string
	}{{0, "Y", "N"}, {1, "N", "Y"}} {
		m.quitConfirmFocused = tc.focus
		content := m.View().Content
		selected := "38;2;255;121;198"
		for _, c := range []struct {
			label string
			want  bool
		}{{tc.selected, true}, {tc.other, false}} {
			pos := strings.Index(content, "m"+c.label+"\x1b[")
			if pos < 0 {
				t.Fatalf("button %s not found", c.label)
			}
			start := strings.LastIndex(content[:pos], "\x1b[")
			if start < 0 || strings.Contains(content[start:pos], selected) != c.want {
				t.Fatalf("focus %d: wrong highlight for %s: %q", tc.focus, c.label, content[start:pos+2])
			}
		}
	}
}
