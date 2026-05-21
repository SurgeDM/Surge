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
		if settings.General.DefaultDownloadDir.AsString() != "" {
			if info, err := os.Stat(settings.General.DefaultDownloadDir.AsString()); err != nil || !info.IsDir() {
				t.Errorf("DefaultDownloadDir set to invalid path: %s", settings.General.DefaultDownloadDir.AsString())
			}
		}

		if !settings.General.WarnOnDuplicate.AsBool() {
			t.Error("WarnOnDuplicate should be true by default")
		}
		if settings.General.AllowRemoteOpenActions.AsBool() {
			t.Error("AllowRemoteOpenActions should be false by default")
		}
		if settings.General.AutoResume.AsBool() {
			t.Error("AutoResume should be false by default")
		}
	})

	// Verify Connection settings
	t.Run("NetworkSettings", func(t *testing.T) {
		if settings.Network.MaxConnectionsPerDownload.AsInt() <= 0 {
			t.Errorf("MaxConnectionsPerDownload should be positive, got: %d", settings.Network.MaxConnectionsPerDownload.AsInt())
		}
		if settings.Network.MaxConnectionsPerDownload.AsInt() > 64 {
			t.Errorf("MaxConnectionsPerDownload shouldn't exceed 64, got: %d", settings.Network.MaxConnectionsPerDownload.AsInt())
		}

		if settings.Network.SequentialDownload.AsBool() {
			t.Error("SequentialDownload should be false by default")
		}
		if settings.Network.DialHedgeCount.AsInt() != 4 {
			t.Errorf("DialHedgeCount should be 4 by default, got: %d", settings.Network.DialHedgeCount.AsInt())
		}
	})

	// Verify Chunk settings
	t.Run("NetworkChunkSettings", func(t *testing.T) {
		if settings.Network.MinChunkSize.AsInt64() <= 0 {
			t.Errorf("MinChunkSize should be positive, got: %d", settings.Network.MinChunkSize.AsInt64())
		}

		if settings.Network.WorkerBufferSize.AsInt() <= 0 {
			t.Errorf("WorkerBufferSize should be positive, got: %d", settings.Network.WorkerBufferSize.AsInt())
		}
	})

	// Verify Performance settings
	t.Run("PerformanceSettings", func(t *testing.T) {
		if settings.Performance.MaxTaskRetries.AsInt() < 0 {
			t.Errorf("MaxTaskRetries should be non-negative, got: %d", settings.Performance.MaxTaskRetries.AsInt())
		}
		if settings.Performance.SlowWorkerThreshold.AsFloat64() < 0 || settings.Performance.SlowWorkerThreshold.AsFloat64() > 1 {
			t.Errorf("SlowWorkerThreshold should be between 0 and 1, got: %f", settings.Performance.SlowWorkerThreshold.AsFloat64())
		}
		if settings.Performance.SlowWorkerGracePeriod.AsDuration() <= 0 {
			t.Errorf("SlowWorkerGracePeriod should be positive, got: %v", settings.Performance.SlowWorkerGracePeriod.AsDuration())
		}
		if settings.Performance.StallTimeout.AsDuration() <= 0 {
			t.Errorf("StallTimeout should be positive, got: %v", settings.Performance.StallTimeout.AsDuration())
		}
		if settings.Performance.SpeedEmaAlpha.AsFloat64() < 0 || settings.Performance.SpeedEmaAlpha.AsFloat64() > 1 {
			t.Errorf("SpeedEmaAlpha should be between 0 and 1, got: %f", settings.Performance.SpeedEmaAlpha.AsFloat64())
		}
	})

	// Verify Extension settings
	t.Run("ExtensionSettings", func(t *testing.T) {
		if !settings.Extension.ExtensionPrompt.AsBool() {
			t.Error("ExtensionPrompt should be true by default")
		}
		if settings.Extension.ChromeExtensionURL.AsString() == "" {
			t.Error("ChromeExtensionURL should not be empty")
		}
		if settings.Extension.FirefoxExtensionURL.AsString() == "" {
			t.Error("FirefoxExtensionURL should not be empty")
		}
		if settings.Extension.InstructionsURL.AsString() == "" {
			t.Error("InstructionsURL should not be empty")
		}
	})
}

func TestDefaultSettings_Consistency(t *testing.T) {
	s1 := DefaultSettings()
	s2 := DefaultSettings()

	if s1 == s2 {
		t.Error("DefaultSettings should return new instance each time")
	}

	if s1.Network.MaxConnectionsPerDownload.AsInt() != s2.Network.MaxConnectionsPerDownload.AsInt() {
		t.Error("Default settings should be consistent")
	}
}

func TestGetSettingsPath(t *testing.T) {
	path := GetSettingsPath()

	if path == "" {
		t.Error("GetSettingsPath returned empty string")
	}

	surgeDir := GetSurgeDir()
	if !strings.HasPrefix(path, surgeDir) {
		t.Errorf("Settings path should be under surge dir. Path: %s, SurgeDir: %s", path, surgeDir)
	}

	if !strings.HasSuffix(path, "settings.json") {
		t.Errorf("Settings path should end with 'settings.json', got: %s", path)
	}
}

func TestSaveAndLoadSettings(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "surge-settings-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	original := DefaultSettings()
	original.General.DefaultDownloadDir.Value = tmpDir
	original.General.WarnOnDuplicate.Value = false
	original.General.AutoResume.Value = true
	original.Network.MaxConnectionsPerDownload.Value = 16
	original.Network.MaxConcurrentDownloads.Value = 7
	original.Network.UserAgent.Value = "TestAgent/1.0"
	original.Network.MinChunkSize.Value = int64(1 * MB)
	original.Network.WorkerBufferSize.Value = 256 * KB
	original.Network.DialHedgeCount.Value = 6
	original.Performance.MaxTaskRetries.Value = 5
	original.Performance.SlowWorkerThreshold.Value = 0.5
	original.Performance.SlowWorkerGracePeriod.Value = 10 * time.Second
	original.Performance.StallTimeout.Value = 5 * time.Second
	original.Performance.SpeedEmaAlpha.Value = 0.5

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal settings: %v", err)
	}

	testPath := filepath.Join(tmpDir, "test_settings.json")
	if err := os.WriteFile(testPath, data, 0o644); err != nil {
		t.Fatalf("Failed to write settings file: %v", err)
	}

	readData, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}

	loaded := DefaultSettings()
	if err := json.Unmarshal(readData, loaded); err != nil {
		t.Fatalf("Failed to unmarshal settings: %v", err)
	}

	if loaded.General.DefaultDownloadDir.AsString() != original.General.DefaultDownloadDir.AsString() {
		t.Errorf("DefaultDownloadDir mismatch: got %q, want %q", loaded.General.DefaultDownloadDir.AsString(), original.General.DefaultDownloadDir.AsString())
	}
	if loaded.General.WarnOnDuplicate.AsBool() != original.General.WarnOnDuplicate.AsBool() {
		t.Error("WarnOnDuplicate mismatch")
	}
	if loaded.Network.MaxConcurrentDownloads.AsInt() != original.Network.MaxConcurrentDownloads.AsInt() {
		t.Error("MaxConcurrentDownloads mismatch")
	}
}

func TestLoadSettings_MissingFile(t *testing.T) {
	settings, err := LoadSettings()
	if err != nil {
		t.Logf("LoadSettings returned error (may be expected): %v", err)
	}

	if settings != nil {
		if settings.Network.MaxConnectionsPerDownload.AsInt() <= 0 {
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

	testPath := filepath.Join(tmpDir, "corrupt.json")
	if err := os.WriteFile(testPath, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	data, _ := os.ReadFile(testPath)
	settings := DefaultSettings()
	err = json.Unmarshal(data, settings)

	if err == nil {
		t.Error("Expected error when unmarshaling invalid JSON")
	}
}

func TestToRuntimeConfig(t *testing.T) {
	settings := DefaultSettings()
	runtime := settings.ToRuntimeConfig()

	if runtime == nil {
		t.Fatal("ToRuntimeConfig returned nil")
	}

	if runtime.MaxConnectionsPerDownload != settings.Network.MaxConnectionsPerDownload.AsInt() {
		t.Error("MaxConnectionsPerDownload not correctly mapped")
	}
}

func TestGetSettingsMetadata(t *testing.T) {
	metadata := GetSettingsMetadata()

	if metadata == nil {
		t.Fatal("GetSettingsMetadata returned nil")
	}

	expectedCategories := CategoryOrder()
	for _, cat := range expectedCategories {
		if _, ok := metadata[cat]; !ok {
			t.Errorf("Missing metadata for category: %s", cat)
		}
	}
}

func TestCategoryOrder(t *testing.T) {
	order := CategoryOrder()

	if len(order) == 0 {
		t.Error("CategoryOrder returned empty slice")
	}
}

func TestSettingsJSON_Serialization(t *testing.T) {
	original := DefaultSettings()

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Start with DefaultSettings() to ensure the struct schema is fully pre-populated
	loaded := DefaultSettings()
	if err := json.Unmarshal(data, loaded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if loaded.Network.MaxConnectionsPerDownload.AsInt() != original.Network.MaxConnectionsPerDownload.AsInt() {
		t.Error("Round-trip failed for MaxConnectionsPerDownload")
	}
}

func TestSaveSettings_RealFunction(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	original := DefaultSettings()
	original.Network.MaxConnectionsPerDownload.Value = 48
	original.General.AutoResume.Value = true

	err := SaveSettings(original)
	if err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	loaded, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	if loaded.Network.MaxConnectionsPerDownload.AsInt() != 48 {
		t.Errorf("MaxConnectionsPerDownload mismatch: got %d, want 48", loaded.Network.MaxConnectionsPerDownload.AsInt())
	}
	if !loaded.General.AutoResume.AsBool() {
		t.Error("AutoResume should be true")
	}
}

func TestSettings_Validate(t *testing.T) {
	defaults := DefaultSettings()

	tests := []struct {
		name     string
		modify   func(*Settings)
		validate func(*testing.T, *Settings)
	}{
		{
			name: "Valid Settings Unchanged",
			modify: func(s *Settings) {
				s.Network.MaxConnectionsPerDownload.Value = 48
				s.General.LogRetentionCount.Value = 10
				s.Performance.SlowWorkerThreshold.Value = 0.5
			},
			validate: func(t *testing.T, s *Settings) {
				if s.Network.MaxConnectionsPerDownload.AsInt() != 48 {
					t.Errorf("Expected 48, got %d", s.Network.MaxConnectionsPerDownload.AsInt())
				}
				if s.General.LogRetentionCount.AsInt() != 10 {
					t.Errorf("Expected 10, got %d", s.General.LogRetentionCount.AsInt())
				}
				if s.Performance.SlowWorkerThreshold.AsFloat64() != 0.5 {
					t.Errorf("Expected 0.5, got %f", s.Performance.SlowWorkerThreshold.AsFloat64())
				}
			},
		},
		{
			name: "Invalid Connections High Reset",
			modify: func(s *Settings) {
				s.Network.MaxConnectionsPerDownload.Value = 999
			},
			validate: func(t *testing.T, s *Settings) {
				if s.Network.MaxConnectionsPerDownload.AsInt() != defaults.Network.MaxConnectionsPerDownload.AsInt() {
					t.Errorf("Expected default %d, got %d", defaults.Network.MaxConnectionsPerDownload.AsInt(), s.Network.MaxConnectionsPerDownload.AsInt())
				}
			},
		},
		{
			name: "Invalid Connections Low Reset",
			modify: func(s *Settings) {
				s.Network.MaxConnectionsPerDownload.Value = 0
			},
			validate: func(t *testing.T, s *Settings) {
				if s.Network.MaxConnectionsPerDownload.AsInt() != defaults.Network.MaxConnectionsPerDownload.AsInt() {
					t.Errorf("Expected default %d, got %d", defaults.Network.MaxConnectionsPerDownload.AsInt(), s.Network.MaxConnectionsPerDownload.AsInt())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := DefaultSettings()
			tt.modify(s)
			s.Validate()
			tt.validate(t, s)
		})
	}
}
