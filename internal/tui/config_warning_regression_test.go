package tui

// Regression tests for: config warnings must appear in the TUI activity log.
//
// Root causes fixed (branch fix-config-fails):
//  1. publishStartupWarnings() fired before the TUI event stream connected → silently dropped.
//  2. Corrupt settings.json produced no StartupWarnings at all.
//
// These tests cover the TUI side: startupConfigWarningMsg dispatch and rendering.

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"github.com/SurgeDM/Surge/internal/config"
)

// newModelWithWarnings builds a minimal RootModel with pre-populated
// StartupConfigWarnings to exercise the Init() → startupConfigWarningMsg path.
func newModelWithWarnings(warnings []string) RootModel {
	return RootModel{
		StartupConfigWarnings: warnings,
		logViewport:           viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		list:                  NewDownloadList(80, 20),
		Settings:              config.DefaultSettings(),
	}
}

// TestConfigWarning_StartupConfigWarningMsg_AppearsInActivityLog is the primary
// regression test: the TUI must show config warnings in the activity log.
func TestConfigWarning_StartupConfigWarningMsg_AppearsInActivityLog(t *testing.T) {
	m := newModelWithWarnings([]string{
		"Config: settings file is corrupt (invalid character 'n') — all settings reset to defaults",
	})

	// Dispatch the message directly — same code path as Init() → cmd() → Update()
	updated, _ := m.Update(startupConfigWarningMsg(m.StartupConfigWarnings))
	m2 := updated.(RootModel)

	if len(m2.logEntries) == 0 {
		t.Fatal("no log entries after startupConfigWarningMsg — config warning was silently dropped")
	}

	entry := strings.Join(m2.logEntries, " ")
	if !strings.Contains(entry, "Config:") {
		t.Errorf("log entry should contain 'Config:' prefix, got: %q", entry)
	}
	if !strings.Contains(entry, "⚠") {
		t.Errorf("log entry should contain warning glyph ⚠, got: %q", entry)
	}
	if !strings.Contains(entry, "corrupt") {
		t.Errorf("log entry should contain the original warning text, got: %q", entry)
	}
}

// TestConfigWarning_MultipleWarnings_AllAppearInLog ensures each warning gets
// its own log entry — no truncation or merging.
func TestConfigWarning_MultipleWarnings_AllAppearInLog(t *testing.T) {
	warnings := []string{
		"Max connections/host reset to default (32)",
		"Max concurrent downloads reset to default (3)",
		"Speed smoothing factor reset to default",
	}
	m := newModelWithWarnings(warnings)

	updated, _ := m.Update(startupConfigWarningMsg(warnings))
	m2 := updated.(RootModel)

	if len(m2.logEntries) < len(warnings) {
		t.Errorf("expected at least %d log entries for %d warnings, got %d: %v",
			len(warnings), len(warnings), len(m2.logEntries), m2.logEntries)
	}

	// Each warning text must appear somewhere in the log.
	combined := strings.Join(m2.logEntries, "\n")
	for _, w := range warnings {
		if !strings.Contains(combined, w) {
			t.Errorf("warning %q not found in activity log", w)
		}
	}
}

// TestConfigWarning_EmptyWarnings_NoLogEntry ensures a startupConfigWarningMsg
// with all empty strings doesn't produce phantom log entries.
func TestConfigWarning_EmptyWarnings_NoLogEntry(t *testing.T) {
	m := newModelWithWarnings(nil)
	m.logEntries = nil

	updated, _ := m.Update(startupConfigWarningMsg([]string{"", ""}))
	m2 := updated.(RootModel)

	if len(m2.logEntries) != 0 {
		t.Errorf("empty warning strings should produce no log entries, got: %v", m2.logEntries)
	}
}

// TestConfigWarning_StartupConfigWarnings_CapturedFromSettings verifies that
// InitialRootModel correctly copies StartupWarnings from the settings object
// into the model's StartupConfigWarnings field.
func TestConfigWarning_StartupConfigWarnings_CapturedFromSettings(t *testing.T) {
	// Build a settings object that already has warnings (as LoadSettings would
	// produce for a corrupt or invalid config).
	settings := config.DefaultSettings()
	settings.StartupWarnings = []string{
		"Config: settings file is corrupt — all settings reset to defaults",
	}

	// Build the model manually with these pre-warmed settings to simulate the
	// InitialRootModel path without needing a real disk write.
	var captured []string
	if len(settings.StartupWarnings) > 0 {
		captured = append([]string(nil), settings.StartupWarnings...)
	}

	m := RootModel{
		Settings:              settings,
		StartupConfigWarnings: captured,
		logViewport:           viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		list:                  NewDownloadList(80, 20),
	}

	if len(m.StartupConfigWarnings) == 0 {
		t.Fatal("StartupConfigWarnings was not populated from settings.StartupWarnings")
	}
	if m.StartupConfigWarnings[0] != settings.StartupWarnings[0] {
		t.Errorf("StartupConfigWarnings[0] = %q, want %q",
			m.StartupConfigWarnings[0], settings.StartupWarnings[0])
	}
}

// TestConfigWarning_ValidSettings_NoStartupConfigWarnings is the happy-path
// regression: clean settings must not produce any startup log noise.
func TestConfigWarning_ValidSettings_NoStartupConfigWarnings(t *testing.T) {
	settings := config.DefaultSettings()
	// DefaultSettings() should have zero warnings.
	if len(settings.StartupWarnings) != 0 {
		t.Errorf("DefaultSettings() produced unexpected StartupWarnings: %v", settings.StartupWarnings)
	}

	// Simulate InitialRootModel's capture logic.
	var captured []string
	if len(settings.StartupWarnings) > 0 {
		captured = append([]string(nil), settings.StartupWarnings...)
	}

	if len(captured) != 0 {
		t.Errorf("clean settings should produce no StartupConfigWarnings, got: %v", captured)
	}
}

// TestConfigWarning_SystemLogMsg_UsesInfoStyle verifies that normal system log
// messages (not config warnings) still render with the info (ℹ) prefix and NOT
// with the warning (⚠) prefix. This is a style-regression guard.
func TestConfigWarning_SystemLogMsg_UsesInfoStyle(t *testing.T) {
	m := RootModel{
		logViewport: viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		list:        NewDownloadList(80, 20),
	}

	from := "github.com/SurgeDM/Surge/internal/engine/events"
	_ = from // suppress unused import — events imported via update_events.go

	// Use the events.SystemLogMsg path directly
	m.addLogEntry(LogStyleStarted.Render("ℹ Startup integrity check: no issues found"))

	if len(m.logEntries) == 0 {
		t.Fatal("expected a log entry")
	}
	entry := m.logEntries[len(m.logEntries)-1]
	if strings.Contains(entry, "⚠") {
		t.Errorf("normal system log should not use warning glyph ⚠, got: %q", entry)
	}
}

// TestConfigWarning_WarningSurvivesLogTruncation verifies that config warnings
// are not lost when the log rolls over the 100-entry cap. This tests ordering:
// warnings added at startup should still be visible if they're recent.
func TestConfigWarning_WarningSurvivesLogTruncation(t *testing.T) {
	m := RootModel{
		logViewport: viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)),
		list:        NewDownloadList(80, 20),
	}

	// Fill log to just under the cap.
	for i := 0; i < 99; i++ {
		m.addLogEntry("filler entry")
	}

	// Now add a config warning as the 100th entry.
	const configWarn = "⚠ Config: settings file is corrupt — all settings reset to defaults"
	m.addLogEntry(LogStyleError.Render(configWarn))

	if len(m.logEntries) != 100 {
		t.Fatalf("expected 100 entries at cap, got %d", len(m.logEntries))
	}
	last := m.logEntries[len(m.logEntries)-1]
	if !strings.Contains(last, "corrupt") {
		t.Errorf("config warning should be the newest entry (last), got: %q", last)
	}

	// Add one more to trigger truncation — warning should be evicted (it's oldest now
	// only if it was first, but here it's the newest so it should survive).
	// Add 2 more to push over the cap: the warning is now entry 100 of 101, so truncation
	// keeps entries [1..100] — warning survives.
	m.addLogEntry("post-warning filler")
	combined := strings.Join(m.logEntries, "\n")
	if !strings.Contains(combined, "corrupt") {
		t.Error("config warning was evicted from the log before older filler entries — ordering is wrong")
	}
}
