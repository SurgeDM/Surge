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
	m := &RootModel{
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
					m.resetSettingToDefault(catName, setting.Key, defaults)

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

	switch field.Kind() {
	case reflect.Bool:
		field.SetBool(!field.Bool())
	case reflect.String:
		field.SetString("modified-value-" + jsonKey)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		field.SetInt(field.Int() + 10)
	case reflect.Int64:
		if field.Type().String() == "time.Duration" {
			field.Set(reflect.ValueOf(field.Interface().(time.Duration) + time.Hour))
		} else {
			field.SetInt(field.Int() + 100)
		}
	case reflect.Float32, reflect.Float64:
		field.SetFloat(field.Float() + 0.5)
	default:
		t.Errorf("Unsupported type for setting %s: %v", jsonKey, field.Kind())
	}
}

// verifyIsDefault checks if a specific setting in the settings struct matches the default value.
func verifyIsDefault(t *testing.T, actual, expected *config.Settings, categoryLabel, jsonKey string) {
	actualField := getFieldByJsonKey(t, actual, categoryLabel, jsonKey)
	expectedField := getFieldByJsonKey(t, expected, categoryLabel, jsonKey)

	if !reflect.DeepEqual(actualField.Interface(), expectedField.Interface()) {
		t.Errorf("Setting %q in category %q was not reset to default.\nGot: %v\nWant: %v",
			jsonKey, categoryLabel, actualField.Interface(), expectedField.Interface())
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
		t.Fatalf("Could not find category: %s", categoryLabel)
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
