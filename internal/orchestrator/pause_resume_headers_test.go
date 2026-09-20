package orchestrator

import (
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestBuildResumeConfig_RestoresPersistedHeaders(t *testing.T) {
	entry := &types.DownloadRecord{
		URL:      "https://example.com/file.zip",
		DestPath: filepath.Join(t.TempDir(), "file.zip"),
		Filename: "file.zip",
		Status:   "paused",
	}
	savedState := &types.DownloadRecord{
		URL:      entry.URL,
		DestPath: entry.DestPath,
		Filename: entry.Filename,
		Headers: map[string]string{
			"Cookie":        "session=resume",
			"Authorization": "Bearer resume-token",
		},
		Tasks: []types.Task{{Offset: 100, Length: 100}},
	}

	cfg := buildResumeConfig("resume-id", t.TempDir(), entry, savedState, config.DefaultSettings())
	if cfg.Headers["Cookie"] != "session=resume" || cfg.Headers["Authorization"] != "Bearer resume-token" {
		t.Fatalf("resumed headers = %v, want persisted credentials", cfg.Headers)
	}

	savedState.Headers["Cookie"] = "session=mutated"
	if cfg.Headers["Cookie"] != "session=resume" {
		t.Fatalf("resumed headers alias saved state: %q", cfg.Headers["Cookie"])
	}
}
