package tui

import (
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
)

func TestSettingsMetadataValidation(t *testing.T) {
	metadata := config.GetSettingsMetadata()
	categories := config.CategoryOrder()

	if len(metadata) == 0 {
		t.Fatal("Expected non-empty settings metadata")
	}

	for _, category := range categories {
		settings, ok := metadata[category]
		if !ok {
			t.Errorf("Category %s missing from metadata", category)
			continue
		}

		if len(settings) == 0 {
			t.Errorf("Category %s has no settings", category)
		}

		for _, s := range settings {
			if s.Key == "" {
				t.Errorf("Setting in category %s has empty Key", category)
			}
			if s.Label == "" {
				t.Errorf("Setting %q in category %s has empty Label", s.Key, category)
			}
			if s.Description == "" {
				t.Errorf("Setting %q in category %s has empty Description", s.Key, category)
			}
			if s.Type == "" {
				t.Errorf("Setting %q in category %s has empty Type", s.Key, category)
			}
		}
	}
}

func TestSettingsFloatResilience(t *testing.T) {
	// Verify that float64 values (e.g. from JSON deserialization) format cleanly
	valInt := formatSettingValue(float64(5), "int", false)
	if valInt != "5" {
		t.Errorf("Expected float64(5) as int to format as \"5\", got %q", valInt)
	}

	valDuration := formatSettingValue(float64(5*time.Second), "duration", false)
	if valDuration != "5s" {
		t.Errorf("Expected float64(5s) as duration to format as \"5s\", got %q", valDuration)
	}
}
