package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/power"
	engineprogress "github.com/SurgeDM/Surge/internal/progress"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

const VisualTestWidth, VisualTestHeight = 160, 56

// VisualTestModel supplies fixture events to the production model. It has no
// layout, color, widget, or rendering implementation of its own.
type VisualTestModel struct{ root RootModel }

func NewVisualTestModel(settings *config.Settings, version string, commit ...string) VisualTestModel {
	InitializeTUI()
	root := InitialRootModel(0, version, nil, nil, settings, false, commit...)
	// Replace only external effects. Settings edits remain in memory, and power
	// controls can be inspected without touching the operating system.
	root.SaveSettingsFunc = nil
	root.powerController = visualTestPower{}
	seedVisualTestData(&root)
	return VisualTestModel{root: root}
}

func seedVisualTestData(root *RootModel) {
	root.activeTab = TabAll
	apply := func(event types.DownloadEvent) {
		updated, _ := root.Update(event)
		*root = updated.(RootModel)
	}
	filenames := []string{"fedora-workstation.iso", "design-assets.zip", "conference-recording.mp4", "project-backup.tar.zst", "game-update.bin", "holiday-photos.zip", "dataset.csv"}
	for i, filename := range filenames {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)
		total := int64(i+1) * 256 * utils.MiB
		transferred := total / 2
		if i == 1 {
			transferred = 0
		}
		if i == 5 {
			transferred = total
		}
		state := visualTestProgress(id, total, transferred)
		started := types.DownloadEvent{
			Type: types.EventStarted, DownloadID: id, URL: "https://example.com/" + filename,
			Filename: filename, DestPath: "/Downloads/" + filename, Total: total,
			State: &types.DownloadRecord{ProgressState: state},
		}
		if i == 0 {
			started.RateLimit, started.RateLimitSet = 24*utils.MiB, true
		}
		apply(started)
		progress := types.DownloadEvent{Type: types.EventProgress, DownloadID: id, Total: total, Downloaded: transferred, Elapsed: time.Duration(30+i*17) * time.Second}
		if i == 0 {
			progress.Speed, progress.Connections = 18*utils.MiB, 8
		}
		apply(progress)
		switch i {
		case 1:
			apply(types.DownloadEvent{Type: types.EventQueued, DownloadID: id})
		case 2:
			apply(types.DownloadEvent{Type: types.EventPaused, DownloadID: id, Downloaded: transferred})
		case 3:
			// Pausing is transient, between a pause request and EventPaused.
			root.FindDownloadByID(id).pausing = true
			state.SetPausing(true)
		case 4:
			apply(types.DownloadEvent{Type: types.EventResumed, DownloadID: id})
			apply(started) // Running again, waiting for transfer progress.
		case 5:
			apply(types.DownloadEvent{Type: types.EventComplete, DownloadID: id, Total: total, Elapsed: progress.Elapsed, AvgSpeed: 21 * utils.MiB})
		case 6:
			apply(types.DownloadEvent{Type: types.EventError, DownloadID: id, Err: errors.New("Server returned 503; resume to retry")})
		}
	}
	// Graph input samples, not pre-rendered bars or colors.
	for i := range root.SpeedHistory {
		root.SpeedHistory[i] = (15 + 7*math.Sin(float64(i)*0.21) + 4*math.Sin(float64(i)*0.67)) * utils.MiB
	}
	root.SpeedHistory[len(root.SpeedHistory)-1] = float64(root.cachedTotalSpeed)
	// Freeze fixture timestamps; styling happens in the normal log renderer.
	for i, entry := range root.logEntries {
		if _, message, ok := strings.Cut(entry, "] "); ok {
			root.logEntries[i] = fmt.Sprintf("[12:34:%02d] %s", i, message)
		}
	}
	root.refreshLogViewportContent()
	root.SelectedDownloadID = root.downloads[0].ID
	root.UpdateListItems()
}

func visualTestProgress(id string, total, downloaded int64) *engineprogress.DownloadProgress {
	const chunkSize = 2 * utils.MiB
	state := engineprogress.New(id, total)
	state.InitBitmap(total, chunkSize)
	count := int((total + chunkSize - 1) / chunkSize)
	partial := make([]int64, count)
	partialCount := 0
	if downloaded > 0 && downloaded < total {
		partialCount = 8
	}
	completed := int((downloaded - int64(partialCount)*chunkSize/2) / chunkSize)
	for i := 0; i < count; i++ {
		switch {
		case i < completed:
			state.SetChunkState(i, types.ChunkCompleted)
		case i < completed+partialCount:
			state.SetChunkState(i, types.ChunkDownloading)
			partial[i] = chunkSize / 2
		}
	}
	state.SetChunkProgress(partial)
	state.Bytes.VerifiedProgress.Store(downloaded)
	return state
}

func (m VisualTestModel) Init() tea.Cmd { return m.root.spinner.Tick }

func (m VisualTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && m.root.state == DashboardState && !m.root.searchActive {
		keys := m.root.keys.Dashboard
		// External actions have no backend in this fixture. Ordinary UI
		// interactions go through RootModel.Update.
		if !key.Matches(k, keys.Up, keys.Down, keys.TabAll, keys.TabQueued, keys.TabActive, keys.TabDone, keys.NextTab, keys.PrevTab, keys.Search, keys.ToggleHelp, keys.Settings, keys.SpeedLimits, keys.Add, keys.Log, keys.PinTab, keys.Quit, keys.ForceQuit, keys.LogUp, keys.LogDown, keys.LogTop, keys.LogBottom, keys.LogClose) && k.String() != "/" {
			return m, nil
		}
	}
	if k, ok := msg.(tea.KeyPressMsg); ok && m.root.state == SettingsState && !m.root.SettingsIsEditing && !m.root.settingsSearchActive && key.Matches(k, m.root.keys.Settings.ReportBug) {
		return m, nil
	}
	updated, cmd := m.root.Update(msg)
	m.root = updated.(RootModel)
	return m, cmd
}

func (m VisualTestModel) Snapshot(width, height int) string {
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = updated.(VisualTestModel)
	m.root.lastResizeTime = time.Time{} // Settled frame after resize debounce.
	return m.View().Content
}

func (m VisualTestModel) View() tea.View { return m.root.View() }

type visualTestPower struct{}

func (visualTestPower) Shutdown(context.Context) error { return nil }
func (visualTestPower) AcquireInhibitor(string) (power.ReleaseFunc, error) {
	return func() error { return nil }, nil
}
