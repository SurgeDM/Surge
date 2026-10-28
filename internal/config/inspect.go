package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// InspectSettings is read-only and rejects corrupt/unknown values instead of
// applying the startup loader's recovery defaults. Sources contains file keys.
func InspectSettings(path string) (*Settings, map[string]bool, error) {
	settings := DefaultSettings()
	sources := make(map[string]bool)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, sources, nil
	}
	if err != nil {
		return nil, nil, errors.New("settings file is unreadable")
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return nil, nil, errors.New("settings file exceeds 1 MiB or is unreadable")
	}
	var raw map[string]any
	if toml.Unmarshal(data, &raw) != nil {
		return nil, nil, errors.New("settings file contains invalid TOML")
	}
	for name, value := range raw {
		table, ok := value.(map[string]any)
		if !ok {
			return nil, nil, errors.New("settings categories must be tables")
		}
		var category *SettingsCategory
		for _, candidate := range settings.CategoriesList {
			if strings.EqualFold(candidate.Name, name) {
				category = candidate
				break
			}
		}
		if category == nil {
			return nil, nil, errors.New("settings file contains an unknown category")
		}
		for key, value := range table {
			if strings.EqualFold(name, "categories") && key == "categories" {
				encoded, err := toml.Marshal(map[string]any{"categories": value})
				if err != nil {
					return nil, nil, errors.New("invalid categories")
				}
				var decoded struct {
					Categories []Category `toml:"categories"`
				}
				if toml.Unmarshal(encoded, &decoded) != nil {
					return nil, nil, errors.New("invalid categories")
				}
				settings.Categories.Categories = decoded.Categories
				continue
			}
			setting := settings.FindSetting(category.Name, key)
			if setting == nil {
				return nil, nil, errors.New("settings file contains an unknown key")
			}
			setting.Value = value
			normalized, err := setting.normalizedValue()
			if err != nil {
				return nil, nil, fmt.Errorf("invalid setting %s.%s", category.Name, setting.Key)
			}
			setting.Value = normalized
			if setting != settings.General.DefaultDownloadDir {
				if err := setting.Validate(normalized); err != nil {
					return nil, nil, fmt.Errorf("invalid setting %s.%s", category.Name, setting.Key)
				}
			}
			sources[strings.ToLower(category.Name+"."+setting.Key)] = true
		}
	}
	for _, category := range settings.Categories.Categories {
		if category.Validate() != nil {
			return nil, nil, errors.New("invalid download category")
		}
	}
	return settings, sources, nil
}
