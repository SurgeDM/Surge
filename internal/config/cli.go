package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseConfigPath splits a string like "General.Auto_Resume" into category and key
func ParseConfigPath(path string) (category string, key string, err error) {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid config path format: expected Category.Key (e.g., General.Auto_Resume)")
	}
	return parts[0], parts[1], nil
}

// GetSetting returns the *Setting object for a given path.
func GetSetting(s *Settings, path string) (*Setting, error) {
	catName, key, err := ParseConfigPath(path)
	if err != nil {
		return nil, err
	}

	for _, cat := range s.CategoriesList {
		if strings.EqualFold(cat.Name, catName) {
			for _, set := range cat.Settings {
				if strings.EqualFold(set.Key, key) {
					return set, nil
				}
			}
			return nil, fmt.Errorf("setting key %q not found in category %q", key, cat.Name)
		}
	}
	return nil, fmt.Errorf("category %q not found", catName)
}

// GetSettingString returns the string representation of a setting.
func GetSettingString(s *Settings, path string) (string, error) {
	set, err := GetSetting(s, path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", set.Value), nil
}

// SetSetting updates a setting from a string input.
func SetSetting(s *Settings, path string, valueStr string) error {
	catName, key, err := ParseConfigPath(path)
	if err != nil {
		return err
	}

	var target *Setting
	for _, cat := range s.CategoriesList {
		if strings.EqualFold(cat.Name, catName) {
			for _, set := range cat.Settings {
				if strings.EqualFold(set.Key, key) {
					target = set
					break
				}
			}
			break
		}
	}

	if target == nil {
		return fmt.Errorf("setting %q not found", path)
	}

	var val any
	switch target.Type {
	case "bool":
		val, err = strconv.ParseBool(valueStr)
		if err != nil {
			return fmt.Errorf("invalid boolean value: %w", err)
		}
	case "int", "int64":
		val, err = strconv.ParseInt(valueStr, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer value: %w", err)
		}
		if target.Type == "int" {
			val = int(val.(int64))
		}
	case "float64":
		val, err = strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return fmt.Errorf("invalid float value: %w", err)
		}
	case "duration":
		val, err = time.ParseDuration(valueStr)
		if err != nil {
			return fmt.Errorf("invalid duration value: %w", err)
		}
	case "string", "auth_token", "link":
		val = valueStr
	default:
		return fmt.Errorf("unsupported setting type %q", target.Type)
	}

	if target.ValidateFunc != nil {
		if err := target.ValidateFunc(val); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	target.Value = val
	return nil
}

// ResetSetting resets a setting to its default value.
func ResetSetting(s *Settings, path string) error {
	catName, key, err := ParseConfigPath(path)
	if err != nil {
		return err
	}

	for _, cat := range s.CategoriesList {
		if strings.EqualFold(cat.Name, catName) {
			for _, set := range cat.Settings {
				if strings.EqualFold(set.Key, key) {
					set.Value = set.DefaultValue
					return nil
				}
			}
			break
		}
	}

	return fmt.Errorf("setting %q not found", path)
}
