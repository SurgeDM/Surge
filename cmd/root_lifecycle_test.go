package cmd

import (
	"testing"

	"github.com/surge-downloader/surge/internal/engine/types"
)

func TestBuildPoolIsNameActive(t *testing.T) {
	getAll := func() []types.DownloadConfig {
		state := types.NewProgressState("dl-2", 0)
		state.SetFilename("from-state.iso")

		return []types.DownloadConfig{
			{Filename: "queued.zip"},
			{DestPath: "/downloads/from-path.mp4"},
			{State: state},
		}
	}

	isNameActive := buildPoolIsNameActive(getAll)
	if isNameActive == nil {
		t.Fatal("expected name activity callback")
	}

	for _, name := range []string{"queued.zip", "from-path.mp4", "from-state.iso"} {
		if !isNameActive(name) {
			t.Fatalf("expected %q to be active", name)
		}
	}

	if isNameActive("missing.bin") {
		t.Fatal("did not expect unrelated filename to be active")
	}
}

func TestNewLocalLifecycleManager_WiresNameActivityCheck(t *testing.T) {
	getAll := func() []types.DownloadConfig {
		return []types.DownloadConfig{{Filename: "active.bin"}}
	}

	mgr := newLocalLifecycleManager(nil, getAll)
	if mgr.IsNameActive == nil {
		t.Fatal("expected IsNameActive to be wired")
	}
	if !mgr.IsNameActive("active.bin") {
		t.Fatal("expected wired IsNameActive callback to inspect active downloads")
	}
}
