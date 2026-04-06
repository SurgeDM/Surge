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

	tests := []struct {
		name     string
		template string
		want     string
	}{
		{"filename", "echo {filename}", "echo 'test.zip'"},
		{"filepath", "mv {filepath} /done/", "mv '/downloads/test.zip' /done/"},
		{"all vars", "{id}: {filename} ({size} bytes, {speed} B/s, {duration})", "'abc123': 'test.zip' (1048576 bytes, 524288.00 B/s, 2s)"},
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
