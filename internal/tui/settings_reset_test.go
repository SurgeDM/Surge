package tui

import (
	"reflect"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
)

func TestSettingsResetExhaustive(t *testing.T) {
	// Initialize a RootModel with default settings
	defaults := config.DefaultSettings()
	m := RootModel{
		Settings: config.DefaultSettings(),
	}

	// metadata map: category label -> list of setting metadata
	metadata := config.GetSettingsMetadata()
	categories := config.CategoryOrder()

	for _, catName := range categories {
		settingsList := metadata[catName]
		t.Run(catName, func(t *testing.T) {
			for _, setting := range settingsList {
				t.Run(setting.Key, func(t *testing.T) {
					// 1. Modify the setting to a non-default value using reflection
					setNonDefaultValue(t, m.Settings, catName, setting.Key)

					// 2. Call reset logic
					if err := m.resetSettingToDefault(catName, setting.Key, defaults); err != nil {
						t.Errorf("Failed to reset setting %s: %v", setting.Key, err)
					}

					// 3. Verify it was reset correctly
					verifyIsDefault(t, m.Settings, defaults, catName, setting.Key)
				})
			}
		})
	}
}

// setNonDefaultValue modifies a specific setting in the settings struct to a known "dirty" value.
func setNonDefaultValue(t *testing.T, s *config.Settings, categoryLabel, jsonKey string) {
	field := getFieldByJsonKey(t, s, categoryLabel, jsonKey)
	setting, ok := field.Interface().(*config.Setting)
	if !ok || setting == nil {
		t.Fatalf("Setting field %s is not a *config.Setting", jsonKey)
	}

	switch val := setting.Value.(type) {
	case bool:
		setting.Value = !val
	case string:
		setting.Value = "modified-value-" + jsonKey
	case int:
		setting.Value = val + 10
	case int64:
		setting.Value = val + 100
	case float64:
		setting.Value = val + 0.5
	case time.Duration:
		setting.Value = val + time.Hour
	default:
		t.Errorf("Unsupported type for setting %s: %T", jsonKey, setting.Value)
	}
}

// verifyIsDefault checks if a specific setting in the settings struct matches the default value.
func verifyIsDefault(t *testing.T, actual, expected *config.Settings, categoryLabel, jsonKey string) {
	actualField := getFieldByJsonKey(t, actual, categoryLabel, jsonKey)
	expectedField := getFieldByJsonKey(t, expected, categoryLabel, jsonKey)

	actSetting, actOk := actualField.Interface().(*config.Setting)
	expSetting, expOk := expectedField.Interface().(*config.Setting)
	if !actOk || !expOk || actSetting == nil || expSetting == nil {
		t.Fatalf("Fields are not *config.Setting")
	}

	if !reflect.DeepEqual(actSetting.Value, expSetting.Value) {
		t.Errorf("Setting %q in category %q was not reset to default.\nGot: %v\nWant: %v",
			jsonKey, categoryLabel, actSetting.Value, expSetting.Value)
	}
}

// getFieldByJsonKey finds the reflect.Value for a setting field based on its UI category and JSON key.
func getFieldByJsonKey(t *testing.T, s *config.Settings, categoryLabel, jsonKey string) reflect.Value {
	v := reflect.ValueOf(s).Elem()

	// Find category struct field
	var catField reflect.Value
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		label := field.Tag.Get("ui_label")
		if label == "" {
			label = field.Name
		}
		if label == categoryLabel {
			catField = v.Field(i)
			break
		}
	}

	if !catField.IsValid() {
		if categoryLabel == "Speed Limits" {
			catField = reflect.ValueOf(s).Elem().FieldByName("Network")
		}
		if !catField.IsValid() {
			t.Fatalf("Could not find category: %s", categoryLabel)
		}
	}

	// Find setting field within category
	for i := 0; i < catField.NumField(); i++ {
		field := catField.Type().Field(i)
		key := field.Tag.Get("json")
		if key == "" {
			key = field.Name
		}
		if key == jsonKey {
			return catField.Field(i)
		}
	}

	t.Fatalf("Could not find setting %s in category %s", jsonKey, categoryLabel)
	return reflect.Value{}
}
