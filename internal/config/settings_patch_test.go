package config

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSettingsPatchMergesEditsAndRejectsConflicts(t *testing.T) {
	baseline := DefaultSettings()
	edited := baseline.Clone()
	edited.Network.MaxConcurrentDownloads.Value = 5
	edited.General.AutoResume.Value = !Resolve[bool](baseline.General.AutoResume)
	patch := DiffSettings(baseline, edited)
	// Exercise JSON numeric coercion at the HTTP boundary.
	data, err := json.Marshal(patch)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &patch); err != nil {
		t.Fatal(err)
	}
	current := baseline.Clone()
	current.Network.GlobalRateLimit.Value = "1024 B/s"
	merged, err := patch.Apply(current)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Network.GlobalRateLimit.Value != "1024 B/s" || merged.Network.MaxConcurrentDownloads.Value != 5 {
		t.Fatalf("edits or concurrent rate update lost: %+v", merged.Network)
	}
	current.Network.MaxConcurrentDownloads.Value = 6
	if _, err := patch.Apply(current); !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if current.General.AutoResume.Value != baseline.General.AutoResume.Value {
		t.Fatal("conflicting patch partially applied")
	}
}

func TestSettingsPatchCategoryRulesAndProtectedValues(t *testing.T) {
	baseline := DefaultSettings()
	edited := baseline.Clone()
	edited.Categories.Categories = nil
	patch := DiffSettings(baseline, edited)
	merged, err := patch.Apply(baseline)
	if err != nil || len(merged.Categories.Categories) != 0 {
		t.Fatalf("cannot clear category rules: %v", err)
	}
	current := baseline.Clone()
	current.Categories.Categories[0].Path = "changed"
	if _, err := patch.Apply(current); !errors.Is(err, ErrSettingsConflict) {
		t.Fatalf("expected category conflict, got %v", err)
	}
	for _, changes := range [][]SettingChange{
		{{Category: "Extension", Key: "auth_token", Before: "", Value: "secret"}},
		{{Category: "General", Key: "missing", Before: false, Value: true}},
		{{Category: "General", Key: "auto_resume", Before: baseline.General.AutoResume.Value, Value: nil}},
		{{Category: "General", Key: "auto_resume", Before: baseline.General.AutoResume.Value, Value: true}, {Category: "General", Key: "auto_resume", Before: baseline.General.AutoResume.Value, Value: true}},
	} {
		if _, err := (SettingsPatch{Changes: changes}).Apply(baseline); err == nil {
			t.Fatalf("accepted invalid patch: %+v", changes)
		}
	}
}
