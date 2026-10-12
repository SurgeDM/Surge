package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/SurgeDM/Surge/internal/config"
)

var errInvalidSettings = errors.New("invalid settings")

// A patch keeps omitted settings intact. Decode it before acquiring the
// lifecycle's settings lock so a slow request body cannot block readers.
func decodeSettingsPatch(r *http.Request) (map[string]map[string]json.RawMessage, error) {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	var patch map[string]map[string]json.RawMessage
	if err := decoder.Decode(&patch); err != nil {
		return nil, err
	}
	if patch == nil {
		return nil, errors.New("settings must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("settings must contain one JSON object")
		}
		return nil, err
	}
	return patch, nil
}

func applySettingsPatch(settings *config.Settings, patch map[string]map[string]json.RawMessage) error {
	for section, fields := range patch {
		var categoryName string
		for _, category := range settings.CategoriesList {
			if strings.EqualFold(category.Name, section) {
				categoryName = category.Name
				break
			}
		}
		if categoryName == "" || fields == nil {
			return fmt.Errorf("invalid settings section %q", section)
		}
		for key, raw := range fields {
			if strings.TrimSpace(string(raw)) == "null" {
				return fmt.Errorf("%s.%s cannot be null", section, key)
			}
			if categoryName == "Categories" && key == "categories" {
				continue
			}
			setting := settings.FindSetting(categoryName, key)
			if setting == nil {
				return fmt.Errorf("unknown setting %s.%s", section, key)
			}
			if err := validateSettingsJSONType(setting.Type, raw); err != nil {
				return fmt.Errorf("%s.%s: %w", section, key, err)
			}
		}
	}

	encoded, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(encoded, settings); err != nil {
		return err
	}
	if problems := settings.Validate(); len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validateSettingsJSONType(kind config.SettingType, raw json.RawMessage) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	switch kind {
	case config.TypeBool:
		if _, ok := value.(bool); ok {
			return nil
		}
	case config.TypeString, config.TypeLink, config.TypeAuthToken:
		if _, ok := value.(string); ok {
			return nil
		}
	case config.TypeInt, config.TypeInt64, config.TypeDuration:
		if number, ok := value.(float64); ok && math.Trunc(number) == number && math.Abs(number) <= 1<<53 {
			return nil
		}
	case config.TypeFloat64:
		if _, ok := value.(float64); ok {
			return nil
		}
	}
	return fmt.Errorf("invalid value for %s setting", kind)
}
