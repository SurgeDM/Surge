package tui

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"
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
