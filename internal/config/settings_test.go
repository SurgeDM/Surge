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
		if Resolve[string](settings.General.DefaultDownloadDir) != "" {
			if info, err := os.Stat(Resolve[string](settings.General.DefaultDownloadDir)); err != nil || !info.IsDir() {
				t.Errorf("DefaultDownloadDir set to invalid path: %s", Resolve[string](settings.General.DefaultDownloadDir))
			}
		}

		if !Resolve[bool](settings.General.WarnOnDuplicate) {
			t.Error("WarnOnDuplicate should be true by default")
		}
		if Resolve[bool](settings.General.AllowRemoteOpenActions) {
			t.Error("AllowRemoteOpenActions should be false by default")
		}
		if Resolve[bool](settings.General.AutoResume) {
			t.Error("AutoResume should be false by default")
		}
	})

	// Verify Connection settings
	t.Run("NetworkSettings", func(t *testing.T) {
		if Resolve[int](settings.Network.MaxConnectionsPerDownload) <= 0 {
			t.Errorf("MaxConnectionsPerDownload should be positive, got: %d", Resolve[int](settings.Network.MaxConnectionsPerDownload))
		}
		if Resolve[int](settings.Network.MaxConnectionsPerDownload) > 64 {
			t.Errorf("MaxConnectionsPerDownload shouldn't exceed 64, got: %d", Resolve[int](settings.Network.MaxConnectionsPerDownload))
		}

		if Resolve[bool](settings.Network.SequentialDownload) {
			t.Error("SequentialDownload should be false by default")
		}
		if Resolve[int](settings.Network.DialHedgeCount) != 4 {
			t.Errorf("DialHedgeCount should be 4 by default, got: %d", Resolve[int](settings.Network.DialHedgeCount))
		}
	})

	// Verify Chunk settings
	t.Run("NetworkChunkSettings", func(t *testing.T) {
		if Resolve[int64](settings.Network.MinChunkSize) <= 0 {
			t.Errorf("MinChunkSize should be positive, got: %d", Resolve[int64](settings.Network.MinChunkSize))
		}

		if Resolve[int](settings.Network.WorkerBufferSize) <= 0 {
			t.Errorf("WorkerBufferSize should be positive, got: %d", Resolve[int](settings.Network.WorkerBufferSize))
		}
	})

	// Verify Performance settings
	t.Run("PerformanceSettings", func(t *testing.T) {
		if Resolve[int](settings.Performance.MaxTaskRetries) < 0 {
			t.Errorf("MaxTaskRetries should be non-negative, got: %d", Resolve[int](settings.Performance.MaxTaskRetries))
		}
		if Resolve[float64](settings.Performance.SlowWorkerThreshold) < 0 || Resolve[float64](settings.Performance.SlowWorkerThreshold) > 1 {
			t.Errorf("SlowWorkerThreshold should be between 0 and 1, got: %f", Resolve[float64](settings.Performance.SlowWorkerThreshold))
		}
		if Resolve[time.Duration](settings.Performance.SlowWorkerGracePeriod) <= 0 {
			t.Errorf("SlowWorkerGracePeriod should be positive, got: %v", Resolve[time.Duration](settings.Performance.SlowWorkerGracePeriod))
		}
		if Resolve[time.Duration](settings.Performance.StallTimeout) <= 0 {
			t.Errorf("StallTimeout should be positive, got: %v", Resolve[time.Duration](settings.Performance.StallTimeout))
		}
		if Resolve[float64](settings.Performance.SpeedEmaAlpha) < 0 || Resolve[float64](settings.Performance.SpeedEmaAlpha) > 1 {
			t.Errorf("SpeedEmaAlpha should be between 0 and 1, got: %f", Resolve[float64](settings.Performance.SpeedEmaAlpha))
		}
	})

	// Verify Extension settings
	t.Run("ExtensionSettings", func(t *testing.T) {
		if !Resolve[bool](settings.Extension.ExtensionPrompt) {
			t.Error("ExtensionPrompt should be true by default")
		}
		if Resolve[string](settings.Extension.ChromeExtensionURL) == "" {
			t.Error("ChromeExtensionURL should not be empty")
		}
		if Resolve[string](settings.Extension.FirefoxExtensionURL) == "" {
			t.Error("FirefoxExtensionURL should not be empty")
		}
		if Resolve[string](settings.Extension.InstructionsURL) == "" {
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

	if Resolve[int](s1.Network.MaxConnectionsPerDownload) != Resolve[int](s2.Network.MaxConnectionsPerDownload) {
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

	if Resolve[string](loaded.General.DefaultDownloadDir) != Resolve[string](original.General.DefaultDownloadDir) {
		t.Errorf("DefaultDownloadDir mismatch: got %q, want %q", Resolve[string](loaded.General.DefaultDownloadDir), Resolve[string](original.General.DefaultDownloadDir))
	}
	if Resolve[bool](loaded.General.WarnOnDuplicate) != Resolve[bool](original.General.WarnOnDuplicate) {
		t.Error("WarnOnDuplicate mismatch")
	}
	if Resolve[int](loaded.Network.MaxConcurrentDownloads) != Resolve[int](original.Network.MaxConcurrentDownloads) {
		t.Error("MaxConcurrentDownloads mismatch")
	}
}

func TestLoadSettings_MissingFile(t *testing.T) {
	settings, err := LoadSettings()
	if err != nil {
		t.Logf("LoadSettings returned error (may be expected): %v", err)
	}

	if settings != nil {
		if Resolve[int](settings.Network.MaxConnectionsPerDownload) <= 0 {
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

	if runtime.MaxConnectionsPerDownload != Resolve[int](settings.Network.MaxConnectionsPerDownload) {
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

	if Resolve[int](loaded.Network.MaxConnectionsPerDownload) != Resolve[int](original.Network.MaxConnectionsPerDownload) {
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

	if Resolve[int](loaded.Network.MaxConnectionsPerDownload) != 48 {
		t.Errorf("MaxConnectionsPerDownload mismatch: got %d, want 48", Resolve[int](loaded.Network.MaxConnectionsPerDownload))
	}
	if !Resolve[bool](loaded.General.AutoResume) {
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
				if Resolve[int](s.Network.MaxConnectionsPerDownload) != 48 {
					t.Errorf("Expected 48, got %d", Resolve[int](s.Network.MaxConnectionsPerDownload))
				}
				if Resolve[int](s.General.LogRetentionCount) != 10 {
					t.Errorf("Expected 10, got %d", Resolve[int](s.General.LogRetentionCount))
				}
				if Resolve[float64](s.Performance.SlowWorkerThreshold) != 0.5 {
					t.Errorf("Expected 0.5, got %f", Resolve[float64](s.Performance.SlowWorkerThreshold))
				}
			},
		},
		{
			name: "Invalid Connections High Reset",
			modify: func(s *Settings) {
				s.Network.MaxConnectionsPerDownload.Value = 999
			},
			validate: func(t *testing.T, s *Settings) {
				if Resolve[int](s.Network.MaxConnectionsPerDownload) != Resolve[int](defaults.Network.MaxConnectionsPerDownload) {
					t.Errorf("Expected default %d, got %d", Resolve[int](defaults.Network.MaxConnectionsPerDownload), Resolve[int](s.Network.MaxConnectionsPerDownload))
				}
			},
		},
		{
			name: "Invalid Connections Low Reset",
			modify: func(s *Settings) {
				s.Network.MaxConnectionsPerDownload.Value = 0
			},
			validate: func(t *testing.T, s *Settings) {
				if Resolve[int](s.Network.MaxConnectionsPerDownload) != Resolve[int](defaults.Network.MaxConnectionsPerDownload) {
					t.Errorf("Expected default %d, got %d", Resolve[int](defaults.Network.MaxConnectionsPerDownload), Resolve[int](s.Network.MaxConnectionsPerDownload))
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

func TestResolveGeneric(t *testing.T) {
	s := DefaultSettings()

	// 1. Verify int
	s.Network.MaxConcurrentDownloads.Value = float64(8)
	if val := Resolve[int](s.Network.MaxConcurrentDownloads); val != 8 {
		t.Errorf("Resolve[int] got %d, want 8", val)
	}

	// 2. Verify bool
	s.General.WarnOnDuplicate.Value = float64(1)
	if val := Resolve[bool](s.General.WarnOnDuplicate); !val {
		t.Errorf("Resolve[bool] got false, want true")
	}

	// 3. Verify string
	s.Network.UserAgent.Value = "Surge/1.0"
	if val := Resolve[string](s.Network.UserAgent); val != "Surge/1.0" {
		t.Errorf("Resolve[string] got %q, want \"Surge/1.0\"", val)
	}

	// 4. Verify duration
	s.Performance.SlowWorkerGracePeriod.Value = "15s"
	if val := Resolve[time.Duration](s.Performance.SlowWorkerGracePeriod); val != 15*time.Second {
		t.Errorf("Resolve[time.Duration] got %v, want 15s", val)
	}
}
