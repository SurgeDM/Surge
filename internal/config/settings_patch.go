package config

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
)

var ErrSettingsConflict = errors.New("settings changed since editing began; reopen settings and retry")

// SettingsPatch carries only edited values and the values the editor started with.
// Category rules are ordered, so they are compared and replaced as a single list.
type SettingsPatch struct {
	Changes    []SettingChange     `json:"changes,omitempty"`
	Categories *CategoryListChange `json:"categories,omitempty"`
}

type SettingChange struct {
	Category string `json:"category"`
	Key      string `json:"key"`
	Before   any    `json:"before"`
	Value    any    `json:"value"`
}

type CategoryListChange struct {
	Before []Category `json:"before"`
	Value  []Category `json:"value"`
}

func DiffSettings(before, updated *Settings) SettingsPatch {
	var patch SettingsPatch
	for _, category := range updated.CategoriesList {
		for _, setting := range category.Settings {
			if setting.Type == TypeAuthToken {
				continue
			}
			previous := before.FindSetting(category.Name, setting.Key)
			if previous != nil && !reflect.DeepEqual(previous.Value, setting.Value) {
				patch.Changes = append(patch.Changes, SettingChange{category.Name, setting.Key, previous.Value, setting.Value})
			}
		}
	}
	if !slices.Equal(before.Categories.Categories, updated.Categories.Categories) {
		patch.Categories = &CategoryListChange{slices.Clone(before.Categories.Categories), slices.Clone(updated.Categories.Categories)}
	}
	return patch
}

func (p SettingsPatch) Empty() bool { return len(p.Changes) == 0 && p.Categories == nil }

// Apply checks every baseline before returning a caller-owned merged snapshot.
// Nothing is published if any edited value conflicts with the current settings.
func (p SettingsPatch) Apply(current *Settings) (*Settings, error) {
	updated := current.Clone()
	seen := make(map[string]bool, len(p.Changes))
	for _, change := range p.Changes {
		setting := updated.FindSetting(change.Category, change.Key)
		key := change.Category + "." + change.Key
		if setting == nil || setting.Type == TypeAuthToken || seen[key] {
			return nil, fmt.Errorf("invalid or duplicate setting %s", key)
		}
		seen[key] = true
		previous := *setting
		previous.Value = change.Before
		before, err := previous.normalizedValue()
		if err != nil {
			return nil, fmt.Errorf("invalid baseline for %s: %w", key, err)
		}
		currentValue, err := setting.normalizedValue()
		if err != nil {
			return nil, fmt.Errorf("invalid current value for %s: %w", key, err)
		}
		if !reflect.DeepEqual(currentValue, before) {
			return nil, fmt.Errorf("%w: %s", ErrSettingsConflict, key)
		}
		setting.Value = change.Value
	}
	if p.Categories != nil {
		if !slices.Equal(current.Categories.Categories, p.Categories.Before) {
			return nil, fmt.Errorf("%w: category rules", ErrSettingsConflict)
		}
		updated.Categories.Categories = slices.Clone(p.Categories.Value)
	}
	if err := updated.NormalizeValues(); err != nil {
		return nil, err
	}
	return updated, nil
}
