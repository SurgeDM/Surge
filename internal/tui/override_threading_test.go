package tui

import (
	"context"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/orchestrator"
	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/types"
)

type overrideMockService struct {
	service.DownloadService
	addFunc func(url, path, filename string, mirrors []string, headers map[string]string, isExplicit bool, workers int, minChunkSize int64) (string, error)
}

func (m *overrideMockService) Add(url, path, filename string, mirrors []string, headers map[string]string, isExplicit bool, workers int, minChunkSize int64) (string, error) {
	if m.addFunc != nil {
		return m.addFunc(url, path, filename, mirrors, headers, isExplicit, workers, minChunkSize)
	}
	return "mock-id", nil
}

func (m *overrideMockService) AddWithID(url string, path string, filename string, mirrors []string, headers map[string]string, id string, workers int, minChunkSize int64) (string, error) {
	if m.addFunc != nil {
		return m.addFunc(url, path, filename, mirrors, headers, false, workers, minChunkSize)
	}
	return id, nil
}

func newOverrideTestModel(t *testing.T, addFunc func(url, path, filename string, mirrors []string, headers map[string]string, isExplicit bool, workers int, minChunkSize int64) (string, error)) RootModel {
	t.Helper()

	bus := orchestrator.NewEventBus()
	mgr := orchestrator.NewLifecycleManager(nil, bus, nil)
	baseSvc := service.NewLocalDownloadService(mgr)
	t.Cleanup(func() { _ = baseSvc.Shutdown() })

	svc := &overrideMockService{
		DownloadService: baseSvc,
		addFunc:         addFunc,
	}

	return RootModel{
		Settings:      config.DefaultSettings(),
		Service:       svc,
		Orchestrator:  nil,
		list:          NewDownloadList(80, 20),
		keys:          config.DefaultKeyMap(),
		inputs:        []textinput.Model{textinput.New(), textinput.New(), textinput.New(), textinput.New()},
		enqueueCtx:    context.Background(),
		cancelEnqueue: func() {},
	}
}

func executeCmds(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			executeCmds(c)
		}
	}
}

func TestOverride_ExtensionConfirmPreservesWorkersAndMinChunkSize(t *testing.T) {
	var capturedWorkers int
	var capturedMinChunkSize int64

	addFunc := func(url, path, filename string, mirrors []string, headers map[string]string, isExplicit bool, workers int, minChunkSize int64) (string, error) {
		capturedWorkers = workers
		capturedMinChunkSize = minChunkSize
		return "real-id", nil
	}

	m := newOverrideTestModel(t, addFunc)
	m.Settings.Extension.ExtensionPrompt.Value = true
	m.Settings.General.WarnOnDuplicate.Value = false

	msg := types.DownloadEvent{
		Type:         types.EventRequest,
		URL:          "http://example.com/file.zip",
		Filename:     "file.zip",
		Path:         t.TempDir(),
		Workers:      8,
		MinChunkSize: 1 << 20,
	}

	updated, _ := m.Update(msg)
	root := updated.(RootModel)

	if root.state != ExtensionConfirmationState {
		t.Fatalf("expected ExtensionConfirmationState, got %v", root.state)
	}
	if root.pendingWorkers != 8 {
		t.Fatalf("pendingWorkers = %d, want 8", root.pendingWorkers)
	}
	if root.pendingMinChunkSize != 1<<20 {
		t.Fatalf("pendingMinChunkSize = %d, want %d", root.pendingMinChunkSize, 1<<20)
	}

	root.inputs[2].SetValue(msg.Path)
	root.inputs[3].SetValue(msg.Filename)

	updated, cmd := root.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	root = updated.(RootModel)
	executeCmds(cmd)

	if capturedWorkers != 8 {
		t.Fatalf("capturedWorkers = %d, want 8", capturedWorkers)
	}
	if capturedMinChunkSize != 1<<20 {
		t.Fatalf("capturedMinChunkSize = %d, want %d", capturedMinChunkSize, 1<<20)
	}
}

func TestOverride_DuplicateContinuePreservesWorkersAndMinChunkSize(t *testing.T) {
	var capturedWorkers int
	var capturedMinChunkSize int64

	addFunc := func(url, path, filename string, mirrors []string, headers map[string]string, isExplicit bool, workers int, minChunkSize int64) (string, error) {
		capturedWorkers = workers
		capturedMinChunkSize = minChunkSize
		return "real-id", nil
	}

	m := newOverrideTestModel(t, addFunc)
	m.Settings.Extension.ExtensionPrompt.Value = false
	m.Settings.General.WarnOnDuplicate.Value = true

	m.downloads = append(m.downloads, &DownloadModel{
		URL:      "http://example.com/file.zip",
		Filename: "file.zip",
	})

	msg := types.DownloadEvent{
		Type:         types.EventRequest,
		URL:          "http://example.com/file.zip",
		Filename:     "file.zip",
		Path:         t.TempDir(),
		Workers:      4,
		MinChunkSize: 512 * 1024,
	}

	updated, _ := m.Update(msg)
	root := updated.(RootModel)

	if root.state != DuplicateWarningState {
		t.Fatalf("expected DuplicateWarningState, got %v", root.state)
	}
	if root.pendingWorkers != 4 {
		t.Fatalf("pendingWorkers = %d, want 4", root.pendingWorkers)
	}
	if root.pendingMinChunkSize != 512*1024 {
		t.Fatalf("pendingMinChunkSize = %d, want %d", root.pendingMinChunkSize, 512*1024)
	}

	updated, cmd := root.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	root = updated.(RootModel)
	executeCmds(cmd)

	if capturedWorkers != 4 {
		t.Fatalf("capturedWorkers = %d, want 4", capturedWorkers)
	}
	if capturedMinChunkSize != 512*1024 {
		t.Fatalf("capturedMinChunkSize = %d, want %d", capturedMinChunkSize, 512*1024)
	}
}

func TestOverride_ManualURLDuplicateDoesNotInheritStaleOverride(t *testing.T) {
	var capturedWorkers int
	var capturedMinChunkSize int64

	addFunc := func(url, path, filename string, mirrors []string, headers map[string]string, isExplicit bool, workers int, minChunkSize int64) (string, error) {
		capturedWorkers = workers
		capturedMinChunkSize = minChunkSize
		return "real-id", nil
	}

	m := newOverrideTestModel(t, addFunc)
	m.Settings.Extension.ExtensionPrompt.Value = false
	m.Settings.General.WarnOnDuplicate.Value = true

	m.pendingWorkers = 16
	m.pendingMinChunkSize = 2 << 20

	m.downloads = append(m.downloads, &DownloadModel{
		URL:      "http://example.com/dup.zip",
		Filename: "dup.zip",
	})

	m.state = InputState
	m.focusedInput = 3
	m.inputs[0].SetValue("http://example.com/dup.zip")
	m.inputs[2].SetValue(t.TempDir())
	m.inputs[3].SetValue("dup.zip")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	root := updated.(RootModel)

	if root.state != DuplicateWarningState {
		t.Fatalf("expected DuplicateWarningState, got %v", root.state)
	}
	if root.pendingWorkers != 0 {
		t.Fatalf("pendingWorkers = %d, want 0 (stale override leaked)", root.pendingWorkers)
	}
	if root.pendingMinChunkSize != 0 {
		t.Fatalf("pendingMinChunkSize = %d, want 0 (stale override leaked)", root.pendingMinChunkSize)
	}

	updated, cmd := root.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	root = updated.(RootModel)
	executeCmds(cmd)

	if capturedWorkers != 0 {
		t.Fatalf("capturedWorkers = %d, want 0 (stale override leaked to engine)", capturedWorkers)
	}
	if capturedMinChunkSize != 0 {
		t.Fatalf("capturedMinChunkSize = %d, want 0 (stale override leaked to engine)", capturedMinChunkSize)
	}
}

func TestOverride_BatchConfirmPreservesWorkersAndMinChunkSize(t *testing.T) {
	var captured []struct {
		workers      int
		minChunkSize int64
	}

	addFunc := func(url, path, filename string, mirrors []string, headers map[string]string, isExplicit bool, workers int, minChunkSize int64) (string, error) {
		captured = append(captured, struct {
			workers      int
			minChunkSize int64
		}{workers, minChunkSize})
		return "real-id-" + url, nil
	}

	m := newOverrideTestModel(t, addFunc)
	m.Settings.General.WarnOnDuplicate.Value = false

	batchPath := t.TempDir()
	batchMsg := types.DownloadEvent{
		Type: types.EventBatchRequest,
		Path: batchPath,
		BatchEvents: []types.DownloadEvent{
			{Type: types.EventRequest, URL: "http://example.com/one.zip", Filename: "one.zip", Path: batchPath, Workers: 2, MinChunkSize: 256 * 1024},
			{URL: "http://example.com/two.zip", Filename: "two.zip", Path: batchPath, Workers: 6, MinChunkSize: 1 << 20},
		},
	}

	updated, _ := m.Update(batchMsg)
	root := updated.(RootModel)

	if root.state != BatchConfirmState {
		t.Fatalf("expected BatchConfirmState, got %v", root.state)
	}
	if len(root.pendingBatchRequests) != 2 {
		t.Fatalf("expected 2 pending batch requests, got %d", len(root.pendingBatchRequests))
	}

	root.inputs[2].SetValue(batchPath)

	updated, cmd := root.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	root = updated.(RootModel)
	executeCmds(cmd)

	if len(captured) != 2 {
		t.Fatalf("expected 2 captured enqueues, got %d", len(captured))
	}
	if captured[0].workers != 2 || captured[0].minChunkSize != 256*1024 {
		t.Fatalf("item 0: workers=%d minChunkSize=%d, want 2 and %d", captured[0].workers, captured[0].minChunkSize, 256*1024)
	}
	if captured[1].workers != 6 || captured[1].minChunkSize != 1<<20 {
		t.Fatalf("item 1: workers=%d minChunkSize=%d, want 6 and %d", captured[1].workers, captured[1].minChunkSize, 1<<20)
	}
}
