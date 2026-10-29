package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SurgeDM/Surge/internal/types"
)

func waitStateHash(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for state hash")
		}
		time.Sleep(time.Millisecond)
	}
}

func blockedStateHasher(t *testing.T) chan struct{} {
	t.Helper()
	waitStateHash(t, func() bool { return len(stateHashSlots) == 0 })
	release := make(chan struct{})
	masterMu.Lock()
	previous := hashFileForState
	hashFileForState = func(context.Context, string, time.Duration) (string, bool, error) {
		// Model an OS read that cannot be interrupted by context cancellation.
		<-release
		return "md5:0123456789abcdef0123456789abcdef", false, nil
	}
	masterMu.Unlock()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		waitStateHash(t, func() bool { return len(stateHashSlots) == 0 })
		masterMu.Lock()
		hashFileForState = previous
		masterMu.Unlock()
	})
	return release
}

func TestStateHashDoesNotBlockCheckpointOrMutateCaller(t *testing.T) {
	dir := setupTestDB(t)
	defer os.RemoveAll(dir)
	defer CloseDB()
	release := blockedStateHasher(t)
	state := &types.DownloadRecord{ID: "async", DestPath: filepath.Join(dir, "file"), Tasks: []types.Task{{Offset: 3, Length: 7}}, Headers: map[string]string{"Test": "original"}, FileHash: "stale"}
	if err := SaveState("url", state.DestPath, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadStates([]string{state.ID})
	if err != nil || loaded[state.ID] == nil || loaded[state.ID].FileHash != "" {
		t.Fatalf("checkpoint unavailable while hashing blocked: %v, %v", loaded, err)
	}
	state.Tasks[0].Offset = 99
	state.Headers["Test"] = "changed"
	close(release)
	waitStateHash(t, func() bool { return len(stateHashSlots) == 0 })
	loaded, err = LoadStates([]string{state.ID})
	if err != nil || loaded[state.ID].FileHash == "" || loaded[state.ID].Tasks[0].Offset != 3 || loaded[state.ID].Headers["Test"] != "original" || state.FileHash != "" {
		t.Fatalf("unexpected hash update: %+v, %v", loaded[state.ID], err)
	}
}

func TestStateHashDiscardsInvalidatedResults(t *testing.T) {
	for _, action := range []string{"replace", "identical-replace", "delete", "tasks", "status", "url", "reconfigure", "close", "remove", "resume-all"} {
		t.Run(action, func(t *testing.T) {
			dir := setupTestDB(t)
			defer os.RemoveAll(dir)
			defer CloseDB()
			release := blockedStateHasher(t)
			state := &types.DownloadRecord{ID: "stale", URL: "url", DestPath: filepath.Join(dir, "file"), Tasks: []types.Task{{Length: 10}}}
			if err := AddToMasterList(types.DownloadRecord{ID: state.ID, URL: state.URL, DestPath: state.DestPath, Status: "paused"}); err != nil {
				t.Fatal(err)
			}
			if err := SaveState("url", state.DestPath, state); err != nil {
				t.Fatal(err)
			}
			var err error
			switch action {
			case "replace":
				state.Downloaded = 9
				err = SaveStateWithOptions("url", state.DestPath, state, SaveStateOptions{SkipFileHash: true})
			case "identical-replace":
				err = SaveStateWithOptions("url", state.DestPath, state, SaveStateOptions{SkipFileHash: true})
			case "delete":
				err = DeleteState(state.ID)
			case "tasks":
				err = DeleteTasks(state.ID)
			case "status":
				err = UpdateStatus(state.ID, "downloading")
			case "url":
				err = UpdateURL(state.ID, "new-url")
			case "reconfigure":
				Configure(filepath.Join(dir, "surge.db")) // even reopening the same path
			case "close":
				CloseDB()
			case "remove":
				err = RemoveFromMasterList(state.ID)
			case "resume-all":
				err = ResumeAllDownloads()
			}
			if err != nil {
				t.Fatal(err)
			}
			close(release)
			waitStateHash(t, func() bool { return len(stateHashSlots) == 0 })
			var ds DetailState
			err = loadGob(mustDetailPath(t, dir, state.ID), &ds)
			if action == "delete" {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("deleted checkpoint resurrected: %v", err)
				}
			} else if err != nil || ds.State.FileHash != "" {
				t.Fatalf("stale hash published: %+v, %v", ds, err)
			}
		})
	}
}

func TestStateHashAdmissionIsBoundedAcrossBackendReset(t *testing.T) {
	dir := setupTestDB(t)
	defer os.RemoveAll(dir)
	defer CloseDB()
	blockedStateHasher(t)
	for _, id := range []string{"one", "two", "three"} {
		Configure(filepath.Join(dir, "surge.db"))
		state := &types.DownloadRecord{ID: id, DestPath: filepath.Join(dir, id)}
		if err := SaveState("url", state.DestPath, state); err != nil {
			t.Fatal(err)
		}
	}
	if len(stateHashSlots) != cap(stateHashSlots) {
		t.Fatalf("admitted %d jobs", len(stateHashSlots))
	}
	loaded, err := LoadStates([]string{"one", "two", "three"})
	if err != nil || len(loaded) != 3 || loaded["three"].FileHash != "" {
		t.Fatalf("saturated hashing lost checkpoint: %v, %v", loaded, err)
	}
}
