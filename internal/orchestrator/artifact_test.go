package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/scheduler"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
)

func TestFinalArtifactPolicyNeverOverwritesRacingDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path+types.IncompleteSuffix, []byte("new bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("existing bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, policy := range []types.ArtifactPolicy{{OnExists: "fail"}, {ExpectedSHA256: strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64), Verification: "verified"}} {
		if err := finalizeArtifact(path, policy); !errors.Is(err, os.ErrExist) {
			t.Fatalf("racing destination accepted with policy %+v: %v", policy, err)
		}
	}
	data, _ := os.ReadFile(path)
	if string(data) != "existing bytes" {
		t.Fatal("destination overwritten")
	}
	data, _ = os.ReadFile(path + types.IncompleteSuffix)
	if string(data) != "new bytes" {
		t.Fatal("working bytes discarded")
	}
}

func TestArtifactPublicationRequiresProofOfRequestedVerification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path+types.IncompleteSuffix, []byte("bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := finalizeArtifact(path, types.ArtifactPolicy{ExpectedSHA256: "expected"}); !errors.Is(err, types.ErrIntegrityMismatch) {
		t.Fatalf("unverified artifact accepted: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("unverified artifact published")
	}
}

func TestPublishedArtifactRemainsCompletedWhenWorkingLinkCleanupFails(t *testing.T) {
	root := setupEventStateDB(t)
	path := filepath.Join(root, "artifact")
	if err := os.WriteFile(path+types.IncompleteSuffix, []byte("published bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	previousRemove := removeCompletedWorkingFile
	removeCompletedWorkingFile = func(string) error { return os.ErrPermission }
	defer func() { removeCompletedWorkingFile = previousRemove }()
	settings := config.DefaultSettings()
	settings.General.DownloadCompleteNotification.Value = false
	bus := NewEventBus()
	manager := NewLifecycleManager(nil, bus, settings)
	defer manager.Shutdown()
	events := make(chan types.DownloadEvent, 2)
	events <- types.DownloadEvent{Type: types.EventStarted, DownloadID: "published", URL: "https://example.com/artifact", DestPath: path, Filename: "artifact", Artifact: types.ArtifactPolicy{OnExists: "fail"}}
	events <- types.DownloadEvent{Type: types.EventComplete, DownloadID: "published", Total: 15, Artifact: types.ArtifactPolicy{OnExists: "fail"}}
	close(events)
	manager.StartEventWorker(events)
	record, err := store.GetDownload("published")
	if err != nil || record == nil || record.Status != "completed" {
		t.Fatalf("cleanup failure regressed publication: %+v %v", record, err)
	}
	// A stale pre-completion checkpoint must also be safe after a crash/restart.
	cfg := types.DownloadRecord{IsResume: true, DestPath: path, Filename: "artifact", OutputPath: root}
	if err := scheduler.RunDownload(context.Background(), &cfg); !errors.Is(err, types.ErrCompleted) {
		t.Fatalf("retained link was resumable: %v", err)
	}
	for _, name := range []string{path, path + types.IncompleteSuffix} {
		data, err := os.ReadFile(name)
		if err != nil || string(data) != "published bytes" {
			t.Fatalf("published bytes changed: %q %v", data, err)
		}
	}
}
