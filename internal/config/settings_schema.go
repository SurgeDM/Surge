package config

// Setting represents a single application configuration option.
// It encapsulates the live value, its default fallback, UI/CLI metadata,
// reboot triggers, and localized validation logic into a unified, self-contained unit.
// This architecture decouples setting definitions from static struct fields,
// allowing dynamic schema resolution, introspection, and centralized validation.
type Setting struct {
	Key          string `json:"key"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	NeedsRestart bool   `json:"needs_restart"`
	Type         string `json:"type"` // "string", "int", "bool", "float64", "duration", "int64", "auth_token", "link"

	Value        any `json:"value"`
	DefaultValue any `json:"default_value"`

	// ValidateFunc is a custom validator for this setting.
	ValidateFunc func(val any) error `json:"-"`
}

// Validate checks the given value against any custom validation rule.
func (s *Setting) Validate(val any) error {
	if s.ValidateFunc != nil {
		return s.ValidateFunc(val)
	}
	return nil
}

// SettingsCategory represents a group of related Setting options.
type SettingsCategory struct {
	Name     string     `json:"name"`
	Settings []*Setting `json:"settings"`
}
