package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
)

func TestConfigCmd_List(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Available Surge Settings:") {
		t.Errorf("expected output to contain 'Available Surge Settings:', got %q", out)
	}
	if !strings.Contains(out, "[General]") {
		t.Errorf("expected output to contain '[General]', got %q", out)
	}
}

func TestConfigCmd_Get(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "general.auto_resume"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Current Value: false") { // Default
		t.Errorf("expected output to contain 'Current Value: false', got %q", out)
	}
}

func TestConfigCmd_SetAndReset(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())

	// Set value
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "general.auto_resume", "true"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Set general.auto_resume to true") {
		t.Errorf("expected output to contain 'Set general.auto_resume to true', got %q", out)
	}

	// Verify persistence
	settings, err := config.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if config.Resolve[bool](settings.General.AutoResume) != true {
		t.Error("expected auto_resume to be persisted as true")
	}

	// Reset value
	buf.Reset()
	rootCmd.SetArgs([]string{"config", "general.auto_resume", "default"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out = buf.String()
	if !strings.Contains(out, "Reset general.auto_resume to default value") {
		t.Errorf("expected output to contain 'Reset general.auto_resume to default value', got %q", out)
	}

	// Verify persistence again
	settings, err = config.LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}
	if config.Resolve[bool](settings.General.AutoResume) != false {
		t.Error("expected auto_resume to be persisted as false (default)")
	}
}

func TestConfigCmd_Open(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AppData", t.TempDir())

	// Create a dummy script to act as the editor
	var dummyEditor string
	if runtime.GOOS == "windows" {
		dummyEditor = filepath.Join(t.TempDir(), "dummy_editor.bat")
		err := os.WriteFile(dummyEditor, []byte("@echo off\r\nexit 0\r\n"), 0755)
		if err != nil {
			t.Fatalf("failed to write dummy editor: %v", err)
		}
	} else {
		dummyEditor = filepath.Join(t.TempDir(), "dummy_editor.sh")
		err := os.WriteFile(dummyEditor, []byte("#!/bin/sh\nexit 0\n"), 0755)
		if err != nil {
			t.Fatalf("failed to write dummy editor: %v", err)
		}
	}

	t.Setenv("EDITOR", dummyEditor)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"config", "open"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
