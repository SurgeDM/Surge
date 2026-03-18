package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultSettings(t *testing.T) {
	settings := DefaultSettings()

	if settings == nil {
		t.Fatal("DefaultSettings returned nil")
	}

	// Verify General settings
	t.Run("GeneralSettings", func(t *testing.T) {
		// DefaultDownloadDir can be empty (for current directory) or a valid path
		if settings.General.DefaultDownloadDir != "" {
			if info, err := os.Stat(settings.General.DefaultDownloadDir); err != nil || !info.IsDir() {
				t.Errorf("DefaultDownloadDir set to invalid path: %s", settings.General.DefaultDownloadDir)
			}
		}

		if !settings.General.WarnOnDuplicate {
			t.Error("WarnOnDuplicate should be true by default")
		}
		if settings.General.ExtensionPrompt {
			t.Error("ExtensionPrompt should be false by default")
		}
		if settings.General.AllowRemoteOpenActions {
			t.Error("AllowRemoteOpenActions should be false by default")
		}
		if settings.General.AutoResume {
			t.Error("AutoResume should be false by default")
		}
	})

	// Verify Connection settings
	t.Run("NetworkSettings", func(t *testing.T) {
		if settings.Network.MaxConnectionsPerHost <= 0 {
			t.Errorf("MaxConnectionsPerHost should be positive, got: %d", settings.Network.MaxConnectionsPerHost)
		}
		if settings.Network.MaxConnectionsPerHost > 64 {
			t.Errorf("MaxConnectionsPerHost shouldn't exceed 64, got: %d", settings.Network.MaxConnectionsPerHost)
		}

		// UserAgent can be empty (means use default)
		if settings.Network.SequentialDownload {
			t.Error("SequentialDownload should be false by default")
		}
	})

	// Verify Chunk settings
	t.Run("NetworkChunkSettings", func(t *testing.T) {
		if settings.Network.MinChunkSize <= 0 {
			t.Errorf("MinChunkSize should be positive, got: %d", settings.Network.MinChunkSize)
		}

		if settings.Network.WorkerBufferSize <= 0 {
			t.Errorf("WorkerBufferSize should be positive, got: %d", settings.Network.WorkerBufferSize)
		}
	})

	// Verify Performance settings
	t.Run("PerformanceSettings", func(t *testing.T) {
		if settings.Performance.MaxTaskRetries < 0 {
			t.Errorf("MaxTaskRetries should be non-negative, got: %d", settings.Performance.MaxTaskRetries)
		}
		if settings.Performance.SlowWorkerThreshold < 0 || settings.Performance.SlowWorkerThreshold > 1 {
			t.Errorf("SlowWorkerThreshold should be between 0 and 1, got: %f", settings.Performance.SlowWorkerThreshold)
		}
		if settings.Performance.SlowWorkerGracePeriod <= 0 {
			t.Errorf("SlowWorkerGracePeriod should be positive, got: %v", settings.Performance.SlowWorkerGracePeriod)
		}
		if settings.Performance.StallTimeout <= 0 {
			t.Errorf("StallTimeout should be positive, got: %v", settings.Performance.StallTimeout)
		}
		if settings.Performance.SpeedEmaAlpha < 0 || settings.Performance.SpeedEmaAlpha > 1 {
			t.Errorf("SpeedEmaAlpha should be between 0 and 1, got: %f", settings.Performance.SpeedEmaAlpha)
		}
	})
}

func TestDefaultSettings_Consistency(t *testing.T) {
	// Multiple calls should return equivalent (but not same pointer) settings
	s1 := DefaultSettings()
	s2 := DefaultSettings()

	if s1 == s2 {
		t.Error("DefaultSettings should return new instance each time")
	}

	// Values should be equal
	if s1.Network.MaxConnectionsPerHost != s2.Network.MaxConnectionsPerHost {
		t.Error("Default settings should be consistent")
	}
}

func TestGetSettingsPath(t *testing.T) {
	path := GetSettingsPath()

	if path == "" {
		t.Error("GetSettingsPath returned empty string")
	}

	// Should be under surge directory
	surgeDir := GetSurgeDir()
	if !strings.HasPrefix(path, surgeDir) {
		t.Errorf("Settings path should be under surge dir. Path: %s, SurgeDir: %s", path, surgeDir)
	}

	// Should end with settings.json
	if !strings.HasSuffix(path, "settings.json") {
		t.Errorf("Settings path should end with 'settings.json', got: %s", path)
	}

	// Should be absolute path
	if !filepath.IsAbs(path) {
		t.Errorf("Settings path should be absolute, got: %s", path)
	}
}

func TestSaveAndLoadSettings(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "surge-settings-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// We'll test the JSON serialization directly since we can't easily mock GetSettingsPath
	original := &Settings{
		General: GeneralSettings{
			DefaultDownloadDir: tmpDir,
			WarnOnDuplicate:    false,
			ExtensionPrompt:    true,
			AutoResume:         true,
		},
		Network: NetworkSettings{
			MaxConnectionsPerHost:  16,
			MaxConcurrentDownloads: 7,
			UserAgent:              "TestAgent/1.0",
			MinChunkSize:           1 * MB,
			WorkerBufferSize:       256 * KB,
		},
		Performance: PerformanceSettings{
			MaxTaskRetries:        5,
			SlowWorkerThreshold:   0.5,
			SlowWorkerGracePeriod: 10 * time.Second,
			StallTimeout:          5 * time.Second,
			SpeedEmaAlpha:         0.5,
		},
	}

	// Serialize to JSON
	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal settings: %v", err)
	}

	// Write to temp file
	testPath := filepath.Join(tmpDir, "test_settings.json")
	if err := os.WriteFile(testPath, data, 0o644); err != nil {
		t.Fatalf("Failed to write settings file: %v", err)
	}

	// Read back
	readData, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}

	loaded := DefaultSettings()
	if err := json.Unmarshal(readData, loaded); err != nil {
		t.Fatalf("Failed to unmarshal settings: %v", err)
	}

	// Verify all fields
	if loaded.General.DefaultDownloadDir != original.General.DefaultDownloadDir {
		t.Errorf("DefaultDownloadDir mismatch: got %q, want %q",
			loaded.General.DefaultDownloadDir, original.General.DefaultDownloadDir)
	}
	if loaded.General.WarnOnDuplicate != original.General.WarnOnDuplicate {
		t.Error("WarnOnDuplicate mismatch")
	}
	if loaded.General.ExtensionPrompt != original.General.ExtensionPrompt {
		t.Error("ExtensionPrompt mismatch")
	}
	if loaded.Network.MaxConcurrentDownloads != original.Network.MaxConcurrentDownloads {
		t.Errorf("MaxConcurrentDownloads mismatch: got %d, want %d", loaded.Network.MaxConcurrentDownloads, original.Network.MaxConcurrentDownloads)
	}
	if loaded.Network.MaxConnectionsPerHost != original.Network.MaxConnectionsPerHost {
		t.Error("MaxConnectionsPerHost mismatch")
	}
	if loaded.Network.UserAgent != original.Network.UserAgent {
		t.Error("UserAgent mismatch")
	}
	if loaded.Network.MinChunkSize != original.Network.MinChunkSize {
		t.Error("MinChunkSize mismatch")
	}
	if loaded.Performance.SlowWorkerGracePeriod != original.Performance.SlowWorkerGracePeriod {
		t.Error("SlowWorkerGracePeriod mismatch")
	}
}

func TestLoadSettings_MissingFile(t *testing.T) {
	// LoadSettings should return defaults when file doesn't exist
	settings, err := LoadSettings()
	if err != nil {
		// Might fail if config dir doesn't exist, which is okay
		t.Logf("LoadSettings returned error (may be expected): %v", err)
	}

	if settings != nil {
		// If we got settings, they should have sensible defaults
		if settings.Network.MaxConnectionsPerHost <= 0 {
			t.Error("Should return default settings with valid values")
		}
	}
}

func TestLoadSettings_CorruptedJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "surge-corrupt-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Write corrupted JSON
	testPath := filepath.Join(tmpDir, "corrupt.json")
	if err := os.WriteFile(testPath, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	// Read and attempt to unmarshal
	data, _ := os.ReadFile(testPath)
	settings := DefaultSettings()
	err = json.Unmarshal(data, settings)

	if err == nil {
		t.Error("Expected error when unmarshaling invalid JSON")
	}
}

func TestLoadSettings_PartialJSON(t *testing.T) {
	// Test that missing fields get filled with defaults
	partial := `{
		"general": {
			"default_download_dir": "/custom/path"
		}
	}`

	settings := DefaultSettings()
	if err := json.Unmarshal([]byte(partial), settings); err != nil {
		t.Fatalf("Failed to unmarshal partial JSON: %v", err)
	}

	// Custom field should be set
	if settings.General.DefaultDownloadDir != "/custom/path" {
		t.Errorf("Custom field not set: %s", settings.General.DefaultDownloadDir)
	}

	// Default field should remain (from the defaults we started with)
	if settings.Network.MaxConnectionsPerHost <= 0 {
		t.Error("Default values should be preserved for missing fields")
	}
}

func TestToRuntimeConfig(t *testing.T) {
	settings := DefaultSettings()
	runtime := settings.ToRuntimeConfig()

	if runtime == nil {
		t.Fatal("ToRuntimeConfig returned nil")
	}

	// Verify all fields are correctly mapped
	if runtime.MaxConnectionsPerHost != settings.Network.MaxConnectionsPerHost {
		t.Error("MaxConnectionsPerHost not correctly mapped")
	}

	if runtime.UserAgent != settings.Network.UserAgent {
		t.Error("UserAgent not correctly mapped")
	}
	if runtime.MinChunkSize != settings.Network.MinChunkSize {
		t.Error("MinChunkSize not correctly mapped")
	}
	if runtime.WorkerBufferSize != settings.Network.WorkerBufferSize {
		t.Error("WorkerBufferSize not correctly mapped")
	}
	if runtime.MaxTaskRetries != settings.Performance.MaxTaskRetries {
		t.Error("MaxTaskRetries not correctly mapped")
	}
	if runtime.SlowWorkerThreshold != settings.Performance.SlowWorkerThreshold {
		t.Error("SlowWorkerThreshold not correctly mapped")
	}
	if runtime.SlowWorkerGracePeriod != settings.Performance.SlowWorkerGracePeriod {
		t.Error("SlowWorkerGracePeriod not correctly mapped")
	}
	if runtime.StallTimeout != settings.Performance.StallTimeout {
		t.Error("StallTimeout not correctly mapped")
	}
	if runtime.SpeedEmaAlpha != settings.Performance.SpeedEmaAlpha {
		t.Error("SpeedEmaAlpha not correctly mapped")
	}
}

func TestGetSettingsMetadata(t *testing.T) {
	metadata := GetSettingsMetadata()

	if metadata == nil {
		t.Fatal("GetSettingsMetadata returned nil")
	}

	// Verify all categories exist
	expectedCategories := CategoryOrder()
	for _, cat := range expectedCategories {
		if _, ok := metadata[cat]; !ok {
			t.Errorf("Missing metadata for category: %s", cat)
		}
	}

	// Verify each metadata entry has required fields
	for category, settings := range metadata {
		for i, setting := range settings {
			if setting.Key == "" {
				t.Errorf("Category %s, index %d: Key is empty", category, i)
			}
			if setting.Label == "" {
				t.Errorf("Category %s, key %s: Label is empty", category, setting.Key)
			}
			if setting.Description == "" {
				t.Errorf("Category %s, key %s: Description is empty", category, setting.Key)
			}
			if setting.Type == "" {
				t.Errorf("Category %s, key %s: Type is empty", category, setting.Key)
			}

			// Verify Type is valid
			validTypes := map[string]bool{
				"string": true, "int": true, "int64": true,
				"bool": true, "duration": true, "float64": true,
			}
			if !validTypes[setting.Type] {
				t.Errorf("Category %s, key %s: Invalid type %q", category, setting.Key, setting.Type)
			}
		}
	}
}

func TestCategoryOrder(t *testing.T) {
	order := CategoryOrder()

	if len(order) == 0 {
		t.Error("CategoryOrder returned empty slice")
	}

	// Should have all expected categories
	expectedCount := 4 // General, Network, Performance, Categories
	if len(order) != expectedCount {
		t.Errorf("Expected %d categories, got %d", expectedCount, len(order))
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for _, cat := range order {
		if seen[cat] {
			t.Errorf("Duplicate category: %s", cat)
		}
		seen[cat] = true
	}

	// Verify order matches metadata keys
	metadata := GetSettingsMetadata()
	for _, cat := range order {
		if _, ok := metadata[cat]; !ok {
			t.Errorf("Category %s in order but not in metadata", cat)
		}
	}
}

func TestSettingsJSON_Serialization(t *testing.T) {
	original := DefaultSettings()

	// Serialize
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Deserialize
	loaded := &Settings{}
	if err := json.Unmarshal(data, loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify round-trip
	if loaded.Network.MaxConnectionsPerHost != original.Network.MaxConnectionsPerHost {
		t.Error("Round-trip failed for MaxConnectionsPerHost")
	}
	if loaded.Performance.StallTimeout != original.Performance.StallTimeout {
		t.Error("Round-trip failed for StallTimeout (duration)")
	}
}

func TestConstants(t *testing.T) {
	// Verify KB and MB constants
	if KB != 1024 {
		t.Errorf("KB should be 1024, got %d", KB)
	}
	if MB != 1024*1024 {
		t.Errorf("MB should be 1048576, got %d", MB)
	}
}

func TestSaveSettings_RealFunction(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	original := DefaultSettings()
	original.Network.MaxConnectionsPerHost = 48
	original.General.AutoResume = true
	original.Network.UserAgent = "TestAgent/3.0"

	err := SaveSettings(original)
	if err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Verify file was created at expected path
	settingsPath := GetSettingsPath()
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		t.Error("Settings file was not created by SaveSettings")
	}

	// Now test LoadSettings to read it back
	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	// Verify values match
	if loaded.Network.MaxConnectionsPerHost != 48 {
		t.Errorf("MaxConnectionsPerHost mismatch: got %d, want 48", loaded.Network.MaxConnectionsPerHost)
	}
	if !loaded.General.AutoResume {
		t.Error("AutoResume should be true")
	}
	if loaded.Network.UserAgent != "TestAgent/3.0" {
		t.Errorf("UserAgent mismatch: got %q, want %q", loaded.Network.UserAgent, "TestAgent/3.0")
	}

	// Cleanup: restore defaults
	_ = SaveSettings(DefaultSettings())
}

func TestLoadSettings_RealFunction(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Test LoadSettings actually reads from disk
	// First save something
	original := DefaultSettings()
	original.Performance.MaxTaskRetries = 99
	err := SaveSettings(original)
	if err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Now load it
	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	if loaded.Performance.MaxTaskRetries != 99 {
		t.Errorf("MaxTaskRetries mismatch: got %d, want 99", loaded.Performance.MaxTaskRetries)
	}

	// Cleanup
	_ = SaveSettings(DefaultSettings())
}

func TestSaveAndLoadSettings_PreservesEmptyCategories(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	settings := DefaultSettings()
	settings.General.Categories = []Category{}

	if err := SaveSettings(settings); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	data, err := os.ReadFile(GetSettingsPath())
	if err != nil {
		t.Fatalf("read settings file: %v", err)
	}
	if !strings.Contains(string(data), `"categories": []`) {
		t.Fatalf("expected explicit empty categories array in settings.json, got: %s", string(data))
	}

	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	if loaded.General.Categories == nil {
		t.Fatal("expected categories slice to be non-nil after load")
	}
	if len(loaded.General.Categories) != 0 {
		t.Fatalf("expected zero categories after reload, got %d", len(loaded.General.Categories))
	}
}

func TestSaveAndLoadSettings_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// Test complete round trip via real functions
	original := &Settings{
		General: GeneralSettings{
			DefaultDownloadDir: "/test/path",
			WarnOnDuplicate:    false,
			ExtensionPrompt:    true,
			AutoResume:         true,
		},
		Network: NetworkSettings{
			MaxConnectionsPerHost: 64,
			UserAgent:             "RoundTripTest/1.0",
			SequentialDownload:    true,
			MinChunkSize:          1 * MB,
			WorkerBufferSize:      1 * MB,
		},
		Performance: PerformanceSettings{
			MaxTaskRetries:        10,
			SlowWorkerThreshold:   0.2,
			SlowWorkerGracePeriod: 15 * time.Second,
			StallTimeout:          10 * time.Second,
			SpeedEmaAlpha:         0.5,
		},
	}

	// Save
	err := SaveSettings(original)
	if err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Load
	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	// Verify all fields
	if loaded.General.WarnOnDuplicate != original.General.WarnOnDuplicate {
		t.Error("WarnOnDuplicate mismatch")
	}
	if loaded.General.ExtensionPrompt != original.General.ExtensionPrompt {
		t.Error("ExtensionPrompt mismatch")
	}

	if loaded.Network.SequentialDownload != original.Network.SequentialDownload {
		t.Error("SequentialDownload mismatch")
	}
	if loaded.Performance.SlowWorkerGracePeriod != original.Performance.SlowWorkerGracePeriod {
		t.Error("SlowWorkerGracePeriod mismatch")
	}

	// Cleanup
	_ = SaveSettings(DefaultSettings())
}

func TestDefaultSettings_Fallback(t *testing.T) {
	// Unset XDG_DOWNLOAD_DIR
	t.Setenv("XDG_DOWNLOAD_DIR", "")

	// We can't easily unset HOME or delete ~/Downloads in a test without affecting the system user or mocking os functions.
	// But we can verify that the result is either empty or a valid directory.
	settings := DefaultSettings()
	dir := settings.General.DefaultDownloadDir

	if dir != "" {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Errorf("DefaultDownloadDir fallback returned invalid path: %s", dir)
		}
	}
}
