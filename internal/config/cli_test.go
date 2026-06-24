package config

import (
	"testing"
	"time"
)

func TestParseConfigPath(t *testing.T) {
	cat, key, err := ParseConfigPath("General.Auto_Resume")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat != "General" || key != "Auto_Resume" {
		t.Errorf("got %q, %q, want General, Auto_Resume", cat, key)
	}

	_, _, err = ParseConfigPath("GeneralAutoResume")
	if err == nil {
		t.Error("expected error for missing dot in path")
	}

	_, _, err = ParseConfigPath("General.")
	if err != nil {
		t.Errorf("unexpected error for empty key: %v", err)
	}
}

func TestGetSetting(t *testing.T) {
	s := DefaultSettings()

	// Valid fetch (case insensitive)
	set, err := GetSetting(s, "general.auto_resume")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if set.Key != "auto_resume" {
		t.Errorf("got key %q, want auto_resume", set.Key)
	}

	// Unknown category
	_, err = GetSetting(s, "Unknown.key")
	if err == nil {
		t.Error("expected error for unknown category")
	}

	// Unknown key
	_, err = GetSetting(s, "General.unknown_key")
	if err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestGetSettingString(t *testing.T) {
	s := DefaultSettings()
	valStr, err := GetSettingString(s, "general.auto_resume")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if valStr != "false" { // Default is false
		t.Errorf("got %q, want false", valStr)
	}
}

func TestSetSetting(t *testing.T) {
	s := DefaultSettings()

	// Boolean
	err := SetSetting(s, "general.auto_resume", "true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if Resolve[bool](s.General.AutoResume) != true {
		t.Error("expected auto_resume to be true")
	}

	// Invalid Boolean
	err = SetSetting(s, "general.auto_resume", "not-a-bool")
	if err == nil {
		t.Error("expected error for invalid boolean")
	}

	// Int
	err = SetSetting(s, "network.max_connections_per_host", "16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if Resolve[int](s.Network.MaxConnectionsPerDownload) != 16 {
		t.Error("expected max_connections_per_host to be 16")
	}

	// Invalid Int
	err = SetSetting(s, "network.max_connections_per_host", "abc")
	if err == nil {
		t.Error("expected error for invalid int")
	}

	// Int64
	err = SetSetting(s, "network.min_chunk_size", "2097152") // 2MB
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if Resolve[int64](s.Network.MinChunkSize) != 2097152 {
		t.Error("expected min_chunk_size to be 2097152")
	}

	// Duration
	err = SetSetting(s, "performance.stall_timeout", "10s")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if Resolve[time.Duration](s.Performance.StallTimeout) != 10*time.Second {
		t.Error("expected stall_timeout to be 10s")
	}
}

func TestResetSetting(t *testing.T) {
	s := DefaultSettings()
	_ = SetSetting(s, "general.auto_resume", "true")

	err := ResetSetting(s, "general.auto_resume")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if Resolve[bool](s.General.AutoResume) != false { // Default is false
		t.Error("expected auto_resume to be reset to false")
	}

	// Unknown setting
	err = ResetSetting(s, "general.unknown")
	if err == nil {
		t.Error("expected error resetting unknown setting")
	}
}
