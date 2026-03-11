package tui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/download"
	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/processing"
)

var errTest = errors.New("test error")

func TestUpdate_ResumeResultSetsResuming(t *testing.T) {
	m := RootModel{
		downloads: []*DownloadModel{
			{ID: "id-1", paused: true, pausing: true, resuming: true},
		},
	}

	updated, _ := m.Update(resumeResultMsg{id: "id-1", err: nil})
	m2 := updated.(RootModel)

	if len(m2.downloads) != 1 {
		t.Fatalf("Expected 1 download, got %d", len(m2.downloads))
	}
	d := m2.downloads[0]
	if d.paused || d.pausing || !d.resuming {
		t.Fatalf("Expected paused/pausing cleared and resuming=true after resumeResultMsg success, got paused=%v pausing=%v resuming=%v", d.paused, d.pausing, d.resuming)
	}
}

func TestUpdate_ResumeResultErrorKeepsFlags(t *testing.T) {
	m := RootModel{
		downloads: []*DownloadModel{
			{ID: "id-1", paused: true, pausing: true, resuming: true},
		},
	}

	updated, _ := m.Update(resumeResultMsg{id: "id-1", err: errTest})
	m2 := updated.(RootModel)
	d := m2.downloads[0]
	if !d.paused || !d.pausing || !d.resuming {
		t.Fatalf("Expected flags unchanged on resumeResultMsg error, got paused=%v pausing=%v resuming=%v", d.paused, d.pausing, d.resuming)
	}
}

func TestUpdate_DownloadStartedKeepsResuming(t *testing.T) {
	dm := NewDownloadModel("id-1", "http://example.com/file", "file", 0)
	dm.paused = true
	dm.pausing = true
	dm.resuming = true
	m := RootModel{
		downloads:   []*DownloadModel{dm},
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(40, 5),
	}

	msg := events.DownloadStartedMsg{
		DownloadID: "id-1",
		URL:        "http://example.com/file",
		Filename:   "file",
		Total:      100,
		DestPath:   "/tmp/file",
		State:      types.NewProgressState("id-1", 100),
	}

	updated, _ := m.Update(msg)
	m2 := updated.(RootModel)
	var d *DownloadModel
	for _, dl := range m2.downloads {
		if dl.ID == "id-1" {
			d = dl
			break
		}
	}
	if d == nil {
		t.Fatal("Expected download id-1 to exist")
	}
	if d.paused || d.pausing || !d.resuming {
		t.Fatalf("Expected paused/pausing cleared and resuming preserved on DownloadStartedMsg, got paused=%v pausing=%v resuming=%v", d.paused, d.pausing, d.resuming)
	}
}

func TestUpdate_EnqueueSuccessMergesOptimisticEntryAfterStart(t *testing.T) {
	optimistic := NewDownloadModel("pending-1", "http://example.com/file", "file.bin", 0)
	optimistic.Destination = "/tmp/file.bin"

	m := RootModel{
		downloads:          []*DownloadModel{optimistic},
		SelectedDownloadID: "pending-1",
		list:               NewDownloadList(80, 20),
		logViewport:        viewport.New(40, 5),
	}

	updated, _ := m.Update(events.DownloadStartedMsg{
		DownloadID: "real-1",
		URL:        "http://example.com/file",
		Filename:   "file.bin",
		Total:      100,
		DestPath:   "/tmp/file.bin",
		State:      types.NewProgressState("real-1", 100),
	})
	m2 := updated.(RootModel)
	if len(m2.downloads) != 2 {
		t.Fatalf("expected optimistic and real entries before enqueue success, got %d", len(m2.downloads))
	}

	updated, _ = m2.Update(enqueueSuccessMsg{
		tempID:   "pending-1",
		id:       "real-1",
		url:      "http://example.com/file",
		path:     "/tmp",
		filename: "file.bin",
	})
	m3 := updated.(RootModel)

	if len(m3.downloads) != 1 {
		t.Fatalf("expected optimistic duplicate to be removed, got %d entries", len(m3.downloads))
	}
	if m3.downloads[0].ID != "real-1" {
		t.Fatalf("remaining download ID = %q, want real-1", m3.downloads[0].ID)
	}
	selected := m3.GetSelectedDownload()
	if selected == nil || selected.ID != "real-1" {
		t.Fatalf("selected download = %#v, want real-1", selected)
	}
}

func TestUpdate_PauseResumeEventsNormalizeFlags(t *testing.T) {
	m := RootModel{
		downloads: []*DownloadModel{
			{ID: "id-1", paused: false, pausing: true, resuming: true},
		},
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(40, 5),
	}

	updated, _ := m.Update(events.DownloadPausedMsg{
		DownloadID: "id-1",
		Filename:   "file",
		Downloaded: 50,
	})
	m2 := updated.(RootModel)
	d := m2.downloads[0]
	if !d.paused || d.pausing || d.resuming {
		t.Fatalf("Expected paused=true and others false after DownloadPausedMsg, got paused=%v pausing=%v resuming=%v", d.paused, d.pausing, d.resuming)
	}

	updated, _ = m2.Update(events.DownloadResumedMsg{
		DownloadID: "id-1",
		Filename:   "file",
	})
	m3 := updated.(RootModel)
	d = m3.downloads[0]
	if d.paused || d.pausing || !d.resuming {
		t.Fatalf("Expected paused/pausing cleared and resuming=true after DownloadResumedMsg, got paused=%v pausing=%v resuming=%v", d.paused, d.pausing, d.resuming)
	}
}

func TestProcessProgressMsg_ClearsResumingOnTransfer(t *testing.T) {
	dm := NewDownloadModel("id-1", "http://example.com/file", "file", 100)
	dm.resuming = true
	dm.Downloaded = 50
	m := RootModel{
		downloads: []*DownloadModel{dm},
		list:      NewDownloadList(80, 20),
	}

	// No transfer yet: keep resuming.
	m.processProgressMsg(events.ProgressMsg{
		DownloadID: "id-1",
		Downloaded: 50,
		Total:      100,
		Speed:      0,
	})
	if !m.downloads[0].resuming {
		t.Fatal("expected resuming=true before transfer starts")
	}

	// Transfer observed: clear resuming.
	m.processProgressMsg(events.ProgressMsg{
		DownloadID: "id-1",
		Downloaded: 60,
		Total:      100,
		Speed:      1024,
	})
	if m.downloads[0].resuming {
		t.Fatal("expected resuming=false after transfer starts")
	}
}

func TestUpdate_DownloadComplete_UsesAverageSpeed(t *testing.T) {
	dm := NewDownloadModel("id-1", "http://example.com/file", "file.bin", 100)
	dm.Speed = 12345 // Simulate last instantaneous speed before completion.
	m := RootModel{
		downloads:   []*DownloadModel{dm},
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(40, 5),
	}

	elapsed := 4 * time.Second
	avgSpeed := float64(26400000) / elapsed.Seconds()
	updated, _ := m.Update(events.DownloadCompleteMsg{
		DownloadID: "id-1",
		Filename:   "file.bin",
		Elapsed:    elapsed,
		Total:      26400000,
		AvgSpeed:   avgSpeed,
	})
	m2 := updated.(RootModel)
	d := m2.downloads[0]

	if !d.done {
		t.Fatal("expected download to be marked done")
	}
	if d.Downloaded != d.Total {
		t.Fatalf("expected downloaded=%d to match total", d.Total)
	}
	if d.Elapsed != elapsed {
		t.Fatalf("elapsed = %v, want %v", d.Elapsed, elapsed)
	}
	if d.Speed != avgSpeed {
		t.Fatalf("speed = %f, want avg speed %f", d.Speed, avgSpeed)
	}
}

func TestUpdate_SettingsIgnoresMissingFourthTab(t *testing.T) {
	m := RootModel{
		state:    SettingsState,
		Settings: config.DefaultSettings(),
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	m2 := updated.(RootModel)

	if m2.SettingsActiveTab >= len(config.CategoryOrder()) {
		t.Fatalf("invalid settings tab index: %d", m2.SettingsActiveTab)
	}

	// Ensure subsequent navigation does not panic with this state.
	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyDown})
	m3 := updated.(RootModel)
	if m3.SettingsActiveTab >= len(config.CategoryOrder()) {
		t.Fatalf("invalid settings tab index after down: %d", m3.SettingsActiveTab)
	}
}

func TestUpdate_DashboardWithNilSettingsDoesNotPanic(t *testing.T) {
	m := RootModel{
		state: DashboardState,
		list:  NewDownloadList(80, 20),
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m2 := updated.(RootModel)
	if m2.Settings == nil {
		t.Fatal("expected default settings to be initialized")
	}
}

func TestUpdate_DownloadRemovedRemovesFromModelAndList(t *testing.T) {
	dm := NewDownloadModel("id-1", "http://example.com/file", "file", 100)
	m := RootModel{
		downloads:   []*DownloadModel{dm},
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(40, 5),
	}
	m.UpdateListItems()

	updated, _ := m.Update(events.DownloadRemovedMsg{
		DownloadID: "id-1",
		Filename:   "file",
	})
	m2 := updated.(RootModel)

	if len(m2.downloads) != 0 {
		t.Fatalf("expected removed download to be absent, got %d entries", len(m2.downloads))
	}

	if len(m2.list.Items()) != 0 {
		t.Fatalf("expected list to be empty after removal, got %d items", len(m2.list.Items()))
	}
}

func TestUpdate_DownloadRemoved_NoOpWhenUnknownID(t *testing.T) {
	dm := NewDownloadModel("id-1", "http://example.com/file", "file", 100)
	m := RootModel{
		downloads:   []*DownloadModel{dm},
		list:        NewDownloadList(80, 20),
		logViewport: viewport.New(40, 5),
	}
	m.UpdateListItems()

	updated, _ := m.Update(events.DownloadRemovedMsg{
		DownloadID: "id-unknown",
		Filename:   "file",
	})
	m2 := updated.(RootModel)

	if len(m2.downloads) != 1 {
		t.Fatalf("expected unknown remove to keep entries, got %d", len(m2.downloads))
	}
}

func TestProcessProgressMsg_UpdatesElapsed(t *testing.T) {
	dm := NewDownloadModel("id-1", "http://example.com/file", "file", 1000)
	m := RootModel{
		downloads: []*DownloadModel{dm},
		list:      NewDownloadList(80, 20),
	}

	elapsed := 12 * time.Second
	m.processProgressMsg(events.ProgressMsg{
		DownloadID: "id-1",
		Downloaded: 400,
		Total:      1000,
		Speed:      1024,
		Elapsed:    elapsed,
	})

	if dm.Elapsed != elapsed {
		t.Fatalf("elapsed = %v, want %v", dm.Elapsed, elapsed)
	}
}

func TestGenerateUniqueFilename_IncompleteSuffixConstant(t *testing.T) {
	// Verify the constant we're using is correct
	if types.IncompleteSuffix != ".surge" {
		t.Errorf("IncompleteSuffix = %q, want .surge", types.IncompleteSuffix)
	}
}

func TestUpdate_DownloadRequestMsg(t *testing.T) {
	// Setup initial model
	ch := make(chan any, 100)
	pool := download.NewWorkerPool(ch, 1)
	svc := core.NewLocalDownloadServiceWithInput(pool, ch)
	t.Cleanup(func() { _ = svc.Shutdown() })

	m := RootModel{
		Settings:    config.DefaultSettings(),
		Service:     svc,
		logViewport: viewport.New(40, 5),
		list:        NewDownloadList(40, 10),
		inputs:      []textinput.Model{textinput.New(), textinput.New(), textinput.New(), textinput.New()},
	}

	// 1. Test Extension Prompt Enabled
	m.Settings.General.ExtensionPrompt = true
	m.Settings.General.WarnOnDuplicate = true

	msg := events.DownloadRequestMsg{
		URL:      "http://example.com/test.zip",
		Filename: "test.zip",
		Path:     "/tmp/downloads",
	}

	newM, _ := m.Update(msg)
	newRoot := newM.(RootModel)

	if newRoot.state != ExtensionConfirmationState {
		t.Errorf("Expected ExtensionConfirmationState, got %v", newRoot.state)
	}
	if newRoot.pendingURL != msg.URL {
		t.Errorf("Expected pendingURL=%s, got %s", msg.URL, newRoot.pendingURL)
	}
	if newRoot.pendingFilename != msg.Filename {
		t.Errorf("Expected pendingFilename=%s, got %s", msg.Filename, newRoot.pendingFilename)
	}
	if newRoot.pendingPath != msg.Path {
		t.Errorf("Expected pendingPath=%s, got %s", msg.Path, newRoot.pendingPath)
	}

	// 2. Test Duplicate Warning (when prompt disabled but duplicate exists)
	m.Settings.General.ExtensionPrompt = false
	m.Settings.General.WarnOnDuplicate = true

	// Add existing download
	m.downloads = append(m.downloads, &DownloadModel{
		URL:      "http://example.com/test.zip",
		Filename: "test.zip",
	})

	newM, _ = m.Update(msg)
	newRoot = newM.(RootModel)

	if newRoot.state != DuplicateWarningState {
		t.Errorf("Expected DuplicateWarningState, got %v", newRoot.state)
	}

	// 3. Test No Prompt (Direct Download)
	m.Settings.General.ExtensionPrompt = false
	m.Settings.General.WarnOnDuplicate = true
	m.downloads = nil // Clear downloads

	// Note: startDownload triggers a command (tea.Cmd), and might update state or lists.
	// Since startDownload also does TUI side effects (addLogEntry), we might just check that
	// it DOESN'T enter a confirmation state.

	newM, _ = m.Update(msg)
	newRoot = newM.(RootModel)

	// Should remain in DashboardState (default) or whatever it was
	if newRoot.state == ExtensionConfirmationState || newRoot.state == DuplicateWarningState {
		t.Errorf("Expected no prompt state, got %v", newRoot.state)
	}
}

func TestStartDownload_UsesProvidedIDWhenServiceSupportsIt(t *testing.T) {
	ch := make(chan any, 16)
	pool := download.NewWorkerPool(ch, 1)
	svc := core.NewLocalDownloadServiceWithInput(pool, ch)
	t.Cleanup(func() {
		_ = svc.Shutdown()
	})

	m := RootModel{
		Settings: config.DefaultSettings(),
		Service:  svc,
		list:     NewDownloadList(80, 20),
		keys:     Keys,
		inputs:   []textinput.Model{textinput.New(), textinput.New(), textinput.New(), textinput.New()},
	}

	requestID := "request-id-123"
	updated, _ := m.startDownload("https://example.com/file.bin", nil, nil, t.TempDir(), false, "file.bin", requestID)

	if len(updated.downloads) != 1 {
		t.Fatalf("expected 1 queued download, got %d", len(updated.downloads))
	}
	if got := updated.downloads[0].ID; got != requestID {
		t.Fatalf("queued download ID = %q, want %q", got, requestID)
	}
}

func TestStartDownload_UsesModelEnqueueContext(t *testing.T) {
	svc := core.NewLocalDownloadServiceWithInput(nil, nil)
	t.Cleanup(func() {
		_ = svc.Shutdown()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	orchestrator := processing.NewLifecycleManager(
		func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
			t.Fatal("enqueue dispatch should not run after context cancellation")
			return "", nil
		},
		nil,
	)

	m := RootModel{
		Settings:      config.DefaultSettings(),
		Service:       svc,
		Orchestrator:  orchestrator,
		enqueueCtx:    ctx,
		cancelEnqueue: func() {},
		list:          NewDownloadList(80, 20),
		logViewport:   viewport.New(40, 5),
	}

	updated, cmd := m.startDownload("https://example.com/file.bin", nil, nil, t.TempDir(), false, "file.bin", "")
	if cmd == nil {
		t.Fatal("expected enqueue command")
	}
	if len(updated.downloads) != 1 {
		t.Fatalf("expected optimistic queued download, got %d", len(updated.downloads))
	}

	msg := cmd()
	errMsg, ok := msg.(enqueueErrorMsg)
	if !ok {
		t.Fatalf("msg = %T, want enqueueErrorMsg", msg)
	}
	if !errors.Is(errMsg.err, context.Canceled) {
		t.Fatalf("err = %v, want context canceled", errMsg.err)
	}
}

func TestStartDownload_GuessesFilenameOptimisticallyWhenProvidedOrInferred(t *testing.T) {
	svc := core.NewLocalDownloadServiceWithInput(nil, nil)
	t.Cleanup(func() {
		_ = svc.Shutdown()
	})

	orchestrator := processing.NewLifecycleManager(
		func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
			return "real-id", nil
		},
		nil,
	)

	targetDir := t.TempDir()
	m := RootModel{
		Settings:     config.DefaultSettings(),
		Service:      svc,
		Orchestrator: orchestrator,
		list:         NewDownloadList(80, 20),
		logViewport:  viewport.New(40, 5),
	}

	updated, _ := m.startDownload("https://example.com/100MB.bin", nil, nil, targetDir, true, "", "")

	if len(updated.downloads) != 1 {
		t.Fatalf("expected 1 optimistic queued download, got %d", len(updated.downloads))
	}
	d := updated.downloads[0]
	if d.Filename != "100MB.bin" {
		t.Fatalf("optimistic filename = %q, want inferred filename", d.Filename)
	}
	if d.Destination != filepath.Join(targetDir, "100MB.bin") {
		t.Fatalf("optimistic destination = %q, want %q", d.Destination, filepath.Join(targetDir, "100MB.bin"))
	}
}

func TestStartDownload_UsesGenericQueuedNameForExplicitFilenameUntilLifecycleConfirms(t *testing.T) {
	svc := core.NewLocalDownloadServiceWithInput(nil, nil)
	t.Cleanup(func() {
		_ = svc.Shutdown()
	})

	orchestrator := processing.NewLifecycleManager(
		func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
			return "real-id", nil
		},
		nil,
	)

	targetDir := t.TempDir()
	m := RootModel{
		Settings:     config.DefaultSettings(),
		Service:      svc,
		Orchestrator: orchestrator,
		list:         NewDownloadList(80, 20),
		logViewport:  viewport.New(40, 5),
	}

	updated, _ := m.startDownload("https://example.com/archive.zip", nil, nil, targetDir, false, "archive.zip", "")

	if len(updated.downloads) != 1 {
		t.Fatalf("expected 1 optimistic queued download, got %d", len(updated.downloads))
	}
	d := updated.downloads[0]
	if d.Filename != "archive.zip" {
		t.Fatalf("optimistic filename = %q, want \"archive.zip\"", d.Filename)
	}
	if d.Destination != filepath.Join(targetDir, "archive.zip") {
		t.Fatalf("optimistic destination = %q, want %q", d.Destination, filepath.Join(targetDir, "archive.zip"))
	}
}

func TestUpdate_EnqueueErrorKeepsFailedDownloadVisibleInDoneTab(t *testing.T) {
	optimistic := NewDownloadModel("pending-1", "http://example.com/file", "file.bin", 0)
	optimistic.Destination = "/tmp/file.bin"

	m := RootModel{
		activeTab:      TabDone,
		downloads:      []*DownloadModel{optimistic},
		list:           NewDownloadList(80, 20),
		logViewport:    viewport.New(40, 5),
		Settings:       config.DefaultSettings(),
		searchQuery:    "",
		categoryFilter: "",
	}

	updated, _ := m.Update(enqueueErrorMsg{tempID: "pending-1", err: errTest})
	m2 := updated.(RootModel)

	if len(m2.downloads) != 1 {
		t.Fatalf("expected failed optimistic entry to remain, got %d entries", len(m2.downloads))
	}
	d := m2.downloads[0]
	if d.ID != "pending-1" {
		t.Fatalf("download ID = %q, want pending-1", d.ID)
	}
	if !d.done {
		t.Fatal("expected enqueue failure to mark the entry done")
	}
	if !errors.Is(d.err, errTest) {
		t.Fatalf("download err = %v, want %v", d.err, errTest)
	}
	if got := len(m2.getFilteredDownloads()); got != 1 {
		t.Fatalf("done tab entries = %d, want 1 failed enqueue entry", got)
	}
}

func TestUpdate_QuitCancelsEnqueueContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	m := RootModel{
		state:         DashboardState,
		keys:          Keys,
		enqueueCtx:    ctx,
		cancelEnqueue: cancel,
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 := updated.(RootModel)

	if !m2.shuttingDown {
		t.Fatal("expected model to enter shutdown state")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected quit to cancel enqueue context")
	}
}

func TestWithEnqueueContext_OverridesStartDownloadContext(t *testing.T) {
	svc := core.NewLocalDownloadServiceWithInput(nil, nil)
	t.Cleanup(func() {
		_ = svc.Shutdown()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	orchestrator := processing.NewLifecycleManager(
		func(string, string, string, []string, map[string]string, bool, int64, bool) (string, error) {
			t.Fatal("enqueue dispatch should not run after shared context cancellation")
			return "", nil
		},
		nil,
	)

	m := InitialRootModel(1700, "test-version", svc, orchestrator, false)
	m = m.WithEnqueueContext(ctx, func() {})

	_, cmd := m.startDownload("https://example.com/file.bin", nil, nil, t.TempDir(), false, "file.bin", "")
	if cmd == nil {
		t.Fatal("expected enqueue command")
	}

	msg := cmd()
	errMsg, ok := msg.(enqueueErrorMsg)
	if !ok {
		t.Fatalf("msg = %T, want enqueueErrorMsg", msg)
	}
	if !errors.Is(errMsg.err, context.Canceled) {
		t.Fatalf("err = %v, want context canceled", errMsg.err)
	}
}

func TestUpdate_RefreshShortcut(t *testing.T) {
	dm := NewDownloadModel("id-1", "http://example.com/file", "file", 100)
	dm.paused = true

	m := RootModel{
		downloads:      []*DownloadModel{dm},
		list:           NewDownloadList(40, 10),
		state:          DashboardState,
		keys:           Keys,
		urlUpdateInput: textinput.New(),
		Service:        core.NewLocalDownloadServiceWithInput(nil, nil),
	}
	m.UpdateListItems()
	m.list.Select(0) // Select the paused download

	// Simulate pressing 'r' (Refresh)
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}

	updated, _ := m.Update(msg)
	newRoot := updated.(RootModel)

	if newRoot.state != URLUpdateState {
		t.Errorf("Expected state to change to URLUpdateState, got %v", newRoot.state)
	}
	if newRoot.urlUpdateInput.Value() != "http://example.com/file" {
		t.Errorf("Expected urlUpdateInput to be pre-filled with 'http://example.com/file', got '%s'", newRoot.urlUpdateInput.Value())
	}
}
