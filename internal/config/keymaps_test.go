package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultKeyMap(t *testing.T) {
	km := DefaultKeyMap()
	if km == nil {
		t.Fatal("DefaultKeyMap returned nil")
	}
	if len(km.Dashboard.Quit.Keys()) == 0 {
		t.Error("Default Dashboard.Quit keys should not be empty")
	}
}

func TestKeyMapConversion(t *testing.T) {
	km := DefaultKeyMap()
	cfg := km.ToConfig()

	if cfg == nil {
		t.Fatal("ToConfig returned nil")
	}

	// Verify some fields
	if len(cfg.Dashboard["Quit"].Keys) == 0 {
		t.Error("Config Dashboard.Quit keys should not be empty")
	}

	// Verify reflection-based conversion
	km2 := DefaultKeyMap()
	// Change a key in config
	cfg.Dashboard["Quit"] = KeyBindingConfig{
		Keys: []string{"ctrl+x"},
		Help: "exit",
	}
	km2.ApplyConfig(cfg)

	if km2.Dashboard.Quit.Keys()[0] != "ctrl+x" {
		t.Errorf("Expected Quit key to be ctrl+x, got %v", km2.Dashboard.Quit.Keys())
	}
	if km2.Dashboard.Quit.Help().Desc != "exit" {
		t.Errorf("Expected Quit help desc to be exit, got %s", km2.Dashboard.Quit.Help().Desc)
	}
}

func TestSaveAndLoadKeyMap(t *testing.T) {
	// Mock SurgeDir
	tmpDir, err := os.MkdirTemp("", "surge-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// We need to override GetSurgeDir or similar if it's used.
	// Since I can't easily override the function, I'll test the inner logic.
	
	km := DefaultKeyMap()
	cfg := km.ToConfig()
	cfg.Dashboard["Quit"] = KeyBindingConfig{
		Keys: []string{"q"},
		Help: "quit app",
	}

	path := filepath.Join(tmpDir, "keymap.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(path, data, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Test loading logic manually since LoadKeyMap uses a fixed path
	var loadedCfg KeyMapConfig
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(data, &loadedCfg)
	if err != nil {
		t.Fatal(err)
	}

	kmLoaded := DefaultKeyMap()
	kmLoaded.ApplyConfig(&loadedCfg)

	if kmLoaded.Dashboard.Quit.Keys()[0] != "q" {
		t.Errorf("Expected loaded Quit key to be q, got %v", kmLoaded.Dashboard.Quit.Keys())
	}
}

func TestValidateKeyMap(t *testing.T) {
	km := &KeyMap{}
	km.Validate()

	defaults := DefaultKeyMap()
	if !reflect.DeepEqual(km.Dashboard, defaults.Dashboard) {
		t.Error("Validate should have filled Dashboard with defaults")
	}
}
