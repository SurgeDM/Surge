package processing

import (
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestExpandTemplate(t *testing.T) {
	ctx := PostActionContext{
		Filename: "test.zip",
		FilePath: "/downloads/test.zip",
		Size:     1048576,
		Speed:    524288.0,
		Duration: 2 * time.Second,
		ID:       "abc123",
		Error:    "",
	}

	// Build expected values using shellEscape so the quoting style matches
	// the current platform (single quotes on Unix, double quotes on Windows).
	filename := shellEscape("test.zip")
	filepath := shellEscape("/downloads/test.zip")
	id := shellEscape("abc123")
	duration := shellEscape("2s")

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{"filename", "echo {filename}", "echo " + filename},
		{"filepath", "mv {filepath} /done/", "mv " + filepath + " /done/"},
		{"all vars", "{id}: {filename} ({size} bytes, {speed} B/s, {duration})", id + ": " + filename + " (1048576 bytes, 524288.00 B/s, " + duration + ")"},
		{"no vars", "echo done", "echo done"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTemplate(tt.template, ctx)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExpandTemplate_ShellEscapeEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		template string
		ctx      PostActionContext
		want     string
	}{
		{
			"filename with spaces and quotes",
			"echo {filename}",
			PostActionContext{Filename: "my file's (1).zip"},
			"echo " + shellEscape("my file's (1).zip"),
		},
		{
			"filename with semicolon (injection attempt)",
			"echo {filename}",
			PostActionContext{Filename: "test; rm -rf /"},
			"echo " + shellEscape("test; rm -rf /"),
		},
		{
			"filepath with dollar sign (env var expansion attempt)",
			"mv {filepath} /out/",
			PostActionContext{FilePath: "/downloads/$HOME/.ssh"},
			"mv " + shellEscape("/downloads/$HOME/.ssh") + " /out/",
		},
		{
			"error with backtick (command substitution attempt)",
			"notify {error}",
			PostActionContext{Error: "failed: `id`"},
			"notify " + shellEscape("failed: `id`"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandTemplate(tt.template, tt.ctx)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRunPostActions_EmptyCommand(t *testing.T) {
	// Should not panic or error with empty commands
	RunPostActions(config.PostDownloadActions{}, PostActionContext{
		Filename: "test.zip",
	}, false)
}

func TestRunPostActions_ValidCommand(t *testing.T) {
	RunPostActions(config.PostDownloadActions{
		OnCompleteCommand: "echo {filename}",
	}, PostActionContext{
		Filename: "test.zip",
	}, false)
}

func TestRunPostActions_ErrorPath(t *testing.T) {
	RunPostActions(config.PostDownloadActions{
		OnErrorCommand: "echo error: {error}",
	}, PostActionContext{
		Filename: "test.zip",
		Error:    "connection reset",
	}, true)
}
