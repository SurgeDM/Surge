package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
)

func TestFilepickerHJumpAndShiftHHome(t *testing.T) {
	for _, batch := range []bool{false, true} {
		dir, home := t.TempDir(), t.TempDir()
		for _, name := range []string{"apple.txt", "hello.txt"} {
			if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		m := RootModel{Settings: config.DefaultSettings(), keys: config.DefaultKeyMap(), filepicker: newFilepicker(dir)}
		m.Settings.General.DefaultDownloadDir.Value = home
		m.filepicker.SetHeight(20)
		m.filepicker, _ = m.filepicker.Update(m.filepicker.Init()())
		press := func(msg tea.KeyPressMsg) {
			var updated tea.Model
			if batch {
				updated, _ = m.updateBatchFilePicker(msg)
			} else {
				updated, _ = m.updateFilePicker(msg)
			}
			m = updated.(RootModel)
		}
		press(tea.KeyPressMsg{Code: 'h', Text: "h"})
		if m.filepicker.CurrentDirectory != dir || filepath.Base(m.filepicker.HighlightedPath()) != "hello.txt" {
			t.Fatalf("batch=%v: lowercase h did not jump to hello.txt", batch)
		}
		press(tea.KeyPressMsg{Code: 'H', Text: "H", Mod: tea.ModShift})
		if m.filepicker.CurrentDirectory != home {
			t.Fatalf("batch=%v: Shift+H did not navigate home", batch)
		}
	}
}

func TestFilepickerJump(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fp-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Errorf("Failed to remove temp dir: %v", err)
		}
	})

	files := []string{"apple.txt", "banana.txt", "cat.txt", "cherry.txt", "dog.txt"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to write file %s: %v", f, err)
		}
	}

	m := RootModel{
		Settings:   config.DefaultSettings(),
		filepicker: newFilepicker(tmpDir),
	}
	m.filepicker.SetHeight(20)

	// filepicker.Init() returns a Cmd that we need to execute to populate the files
	cmd := m.filepicker.Init()
	if cmd != nil {
		msg := cmd()
		m.filepicker, _ = m.filepicker.Update(msg)
	}

	// Helper to check current file
	checkCurrent := func(expected string) {
		t.Helper()
		base := filepath.Base(m.filepicker.HighlightedPath())
		if base != expected {
			t.Errorf("Expected %s, got %s", expected, base)
		}
	}

	// Depending on OS and filepicker sort, it might not be 'apple.txt' first if there are hidden files, etc.
	// But in a fresh temp dir, it should be apple.txt
	checkCurrent("apple.txt")

	// Jump to 'c'
	m, handled := m.handleFilepickerJump("c")
	if !handled {
		t.Error("Expected jump to 'c' to be handled")
	}
	checkCurrent("cat.txt")

	// Jump to 'c' again (should go to cherry.txt)
	m, handled = m.handleFilepickerJump("c")
	if !handled {
		t.Error("Expected second jump to 'c' to be handled")
	}
	checkCurrent("cherry.txt")

	// Jump to 'c' again (should wrap around to cat.txt)
	m, handled = m.handleFilepickerJump("c")
	if !handled {
		t.Error("Expected wrap around jump to 'c' to be handled")
	}
	checkCurrent("cat.txt")

	// Jump to 'd'
	m, handled = m.handleFilepickerJump("d")
	if !handled {
		t.Error("Expected jump to 'd' to be handled")
	}
	checkCurrent("dog.txt")

	// Jump to 'z' (not found)
	m, handled = m.handleFilepickerJump("z")
	if handled {
		t.Error("Expected jump to 'z' to not be handled")
	}
	// Cursor should remain at dog.txt
	checkCurrent("dog.txt")
}
