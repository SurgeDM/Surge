package tui

import (
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	"github.com/SurgeDM/Surge/internal/config"
)

type helperKeyMap interface {
	ShortHelp() []key.Binding
	FullHelp() [][]key.Binding
}

func testKeyMapInHelp(t *testing.T, name string, km helperKeyMap, ignored map[string]bool) {
	v := reflect.ValueOf(km)
	typ := v.Type()

	// Collect all bindings from FullHelp and ShortHelp
	helpBindings := make(map[string]bool)
	for _, b := range km.ShortHelp() {
		helpBindings[b.Help().Key] = true
	}
	for _, row := range km.FullHelp() {
		for _, b := range row {
			helpBindings[b.Help().Key] = true
		}
	}

	for i := 0; i < v.NumField(); i++ {
		fieldName := typ.Field(i).Name
		field := v.Field(i)

		if field.Type() == reflect.TypeOf(key.Binding{}) {
			binding := field.Interface().(key.Binding)

			// Skip if explicitly ignored
			if ignored[fieldName] {
				continue
			}

			// Check if it has help text. If no help text is defined, we assume it's intentionally hidden from help.
			if binding.Help().Key == "" {
				continue
			}

			if !helpBindings[binding.Help().Key] {
				t.Errorf("%s: Keybinding %s (key: %s) is defined but missing from Help (ShortHelp or FullHelp)", name, fieldName, binding.Help().Key)
			}
		}
	}
}

func TestDashboardKeyMap_AllKeysInHelp(t *testing.T) {
	ignored := map[string]bool{
		"Up":        true, // Basic navigation
		"Down":      true, // Basic navigation
		"LogUp":     true, // Log navigation (only when log is focused)
		"LogDown":   true, // Log navigation
		"LogTop":    true, // Log navigation
		"LogBottom": true, // Log navigation
		"LogClose":  true, // Log navigation
		"ForceQuit": true, // Internal/Alternative quit
	}
	testKeyMapInHelp(t, "Dashboard", Keys.Dashboard, ignored)
}

func TestInputKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "Input", Keys.Input, map[string]bool{
		"Up":   true,
		"Down": true,
	})
}

func TestFilePickerKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "FilePicker", Keys.FilePicker, nil)
}

func TestSettingsKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "Settings", Keys.Settings, nil)
}

func TestCategoryManagerKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "CategoryMgr", Keys.CategoryMgr, nil)
}

func TestQuitConfirmKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "QuitConfirm", Keys.QuitConfirm, nil)
}

func TestDuplicateKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "Duplicate", Keys.Duplicate, nil)
}

func TestExtensionKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "Extension", Keys.Extension, nil)
}

func TestSettingsEditorKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "SettingsEditor", Keys.SettingsEditor, nil)
}

func TestBatchConfirmKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "BatchConfirm", Keys.BatchConfirm, nil)
}

func TestUpdateKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "Update", Keys.Update, nil)
}

func TestBugReportKeyMap_AllKeysInHelp(t *testing.T) {
	testKeyMapInHelp(t, "BugReport", Keys.BugReport, nil)
}

func TestDynamicKeyMapReloading(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: GetSurgeDir uses %APPDATA% and does not honor XDG_CONFIG_HOME")
	}

	tmpDir, err := os.MkdirTemp("", "surge-tui-keymap-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	// Override configuration directory
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}()
	_ = os.Setenv("XDG_CONFIG_HOME", tmpDir)

	err = config.EnsureDirs()
	if err != nil {
		t.Fatalf("Failed to ensure directories: %v", err)
	}

	// 1. Initialize keymap and verify default state
	km, err := LoadKeyMap()
	if err != nil {
		t.Fatalf("Failed to load keymap: %v", err)
	}

	m := RootModel{
		keys:                km,
		lastKeyMapModTime:   time.Now().Add(-10 * time.Second), // Ensure modTime is older
		lastConfigCheckTime: time.Now().Add(-2 * time.Second),  // Ensure check triggers
	}

	if len(m.keys.Dashboard.ToggleHelp.Keys()) != 1 || m.keys.Dashboard.ToggleHelp.Keys()[0] != "/" {
		t.Errorf("Expected default ToggleHelp key '/', got %v", m.keys.Dashboard.ToggleHelp.Keys())
	}

	// 2. Simulate user editing keymap.json on disk
	customKeyMap := DefaultKeyMap()
	customKeyMap.Dashboard.ToggleHelp = key.NewBinding(
		key.WithKeys("ctrl+x"),
		key.WithHelp("ctrl+x", "keybindings"),
	)

	// Save custom keymap to temp directory
	err = SaveKeyMap(customKeyMap)
	if err != nil {
		t.Fatalf("Failed to save custom keymap: %v", err)
	}

	// Update modTime on disk to simulate fresh write in the past
	keymapPath := GetKeyMapConfigPath()
	now := time.Now()
	err = os.Chtimes(keymapPath, now, now)
	if err != nil {
		t.Fatalf("Failed to set file times: %v", err)
	}

	// 3. Trigger TUI update loop and assert dynamic reload
	res, _ := m.Update(struct{}{})
	updatedModel := res.(RootModel)

	// Ensure the new custom keybinding was hot-reloaded dynamically
	toggleHelpKeys := updatedModel.keys.Dashboard.ToggleHelp.Keys()
	if len(toggleHelpKeys) != 1 || toggleHelpKeys[0] != "ctrl+x" {
		t.Errorf("Expected dynamic reload to update ToggleHelp key to 'ctrl+x', got %v", toggleHelpKeys)
	}
}

func TestDynamicKeyMapReloading_PostLoadStatFailureUsesCurrentTime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: GetSurgeDir uses %APPDATA% and does not honor XDG_CONFIG_HOME")
	}

	tmpDir, err := os.MkdirTemp("", "surge-tui-keymap-stat-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)
	}()
	_ = os.Setenv("XDG_CONFIG_HOME", tmpDir)

	if err := config.EnsureDirs(); err != nil {
		t.Fatalf("Failed to ensure directories: %v", err)
	}
	if err := SaveKeyMap(DefaultKeyMap()); err != nil {
		t.Fatalf("Failed to save keymap: %v", err)
	}

	keymapPath := GetKeyMapConfigPath()
	staleModTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(keymapPath, staleModTime, staleModTime); err != nil {
		t.Fatalf("Failed to set file times: %v", err)
	}

	originalStat := keyMapConfigStat
	statCalls := 0
	keyMapConfigStat = func(name string) (os.FileInfo, error) {
		statCalls++
		if statCalls == 2 {
			return nil, os.ErrNotExist
		}
		return originalStat(name)
	}
	t.Cleanup(func() {
		keyMapConfigStat = originalStat
	})

	m := RootModel{
		keys:                DefaultKeyMap(),
		lastKeyMapModTime:   staleModTime.Add(-time.Second),
		lastConfigCheckTime: time.Now().Add(-2 * time.Second),
	}

	beforeUpdate := time.Now().Add(-time.Millisecond)
	res, _ := m.Update(struct{}{})
	updatedModel := res.(RootModel)

	if statCalls != 2 {
		t.Fatalf("Expected keymap stat to be called twice, got %d", statCalls)
	}
	if updatedModel.lastKeyMapModTime.Before(beforeUpdate) {
		t.Fatalf("Expected fallback modtime to use current time, got %s before %s", updatedModel.lastKeyMapModTime, beforeUpdate)
	}
}
