package tui

import (
	"fmt"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"strings"
	"testing"
)

func TestTransparentBackgroundPreservesControls(t *testing.T) {
	InitializeTUI()
	m := InitialRootModel(1701, "test", nil, nil, config.DefaultSettings(), false)
	m.width, m.height, m.state = 100, 30, QuitConfirmState
	m.Settings.General.TransparentBackground.Value = true
	modal := m.viewQuitConfirm()
	view := m.View()
	if view.BackgroundColor != nil {
		t.Fatal("transparent view sets terminal background")
	}
	// Control styles must survive unchanged when placed on a transparent surface.
	if !strings.Contains(modal, "48;2;255;121;198") || !strings.Contains(view.Content, "48;2;255;121;198") {
		t.Fatal("transparent overlay changed modal controls")
	}
	selection := colorToRGB(colors.SelectionBackground())
	inactiveBackground := fmt.Sprintf("48;2;%d;%d;%d", selection.r, selection.g, selection.b)
	if !strings.Contains(modal, inactiveBackground) {
		t.Fatal("transparent overlay removed the inactive Nope button fill")
	}
	m.Settings.General.TransparentBackground.Value = false
	if m.View().BackgroundColor == nil {
		t.Fatal("opaque view has no themed background")
	}
}
