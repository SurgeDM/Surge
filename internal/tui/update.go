package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/surge-downloader/surge/internal/processing"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/utils"
	"github.com/surge-downloader/surge/internal/version"

	tea "charm.land/bubbletea/v2"
)

// notificationTickMsg is sent to check if a notification should be cleared
type notificationTickMsg struct{}

// UpdateCheckResultMsg is sent when the update check is complete
type UpdateCheckResultMsg struct {
	Info *version.UpdateInfo
}

type shutdownCompleteMsg struct {
	err error
}

type enqueueSuccessMsg struct {
	tempID   string
	id       string
	url      string
	path     string
	filename string
}

type enqueueErrorMsg struct {
	tempID string
	err    error
}

// checkForUpdateCmd performs an async update check
func checkForUpdateCmd(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		info, _ := version.CheckForUpdate(currentVersion)
		return UpdateCheckResultMsg{Info: info}
	}
}

func shutdownCmd(service interface{ Shutdown() error }) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return shutdownCompleteMsg{}
		}
		return shutdownCompleteMsg{err: service.Shutdown()}
	}
}

// openWithSystem opens a file or URL with the system's default application
func openWithSystem(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	default: // linux and others
		cmd = exec.Command("xdg-open", path)
	}
	err := cmd.Start()
	if err == nil {
		go func() {
			_ = cmd.Wait()
		}()
	}
	return err
}

// addLogEntry adds a log entry to the log viewport
func (m *RootModel) addLogEntry(msg string) {
	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", timestamp, msg)
	m.logEntries = append(m.logEntries, entry)

	// Keep only the last 100 entries to prevent memory issues
	if len(m.logEntries) > 100 {
		m.logEntries = m.logEntries[len(m.logEntries)-100:]
	}

	// Update viewport content
	m.logViewport.SetContent(strings.Join(m.logEntries, "\n"))
	// Auto-scroll to bottom
	m.logViewport.GotoBottom()
}

// removeDownloadByID removes a download from the in-memory list.
// Returns true if a download was removed.
func (m *RootModel) removeDownloadByID(id string) bool {
	for i, d := range m.downloads {
		if d.ID == id {
			m.downloads = append(m.downloads[:i], m.downloads[i+1:]...)
			return true
		}
	}
	return false
}

func (m *RootModel) handleFilePickerSelection(path string) (tea.Model, tea.Cmd) {
	if m.SettingsFileBrowsing {
		m.Settings.General.DefaultDownloadDir = path
		m.SettingsFileBrowsing = false
		m.state = SettingsState
		return m, nil
	}
	if m.ExtensionFileBrowsing {
		m.inputs[2].SetValue(path)
		m.ExtensionFileBrowsing = false
		m.state = ExtensionConfirmationState
		return m, nil
	}
	if m.catMgrFileBrowsing {
		m.catMgrInputs[3].SetValue(path)
		m.catMgrFileBrowsing = false
		m.state = CategoryManagerState
		return m, nil
	}
	m.inputs[2].SetValue(path)
	m.state = InputState
	return m, nil
}

func (m *RootModel) handleFilePickerGotoHome() tea.Cmd {
	defaultDir := m.Settings.General.DefaultDownloadDir
	if defaultDir == "" {
		homeDir, _ := os.UserHomeDir()
		defaultDir = filepath.Join(homeDir, "Downloads")
	}
	m.filepicker = newFilepicker(defaultDir)
	return m.filepicker.Init()
}

func (m *RootModel) resetFilepickerToDirMode() {
	m.filepicker.FileAllowed = false
	m.filepicker.DirAllowed = true
	m.filepicker.AllowedTypes = nil
}

// checkForDuplicate checks if a compatible download already exists
func (m RootModel) checkForDuplicate(url string) *processing.DuplicateResult {
	activeDownloads := func() map[string]*types.DownloadConfig {
		active := make(map[string]*types.DownloadConfig)
		for _, d := range m.downloads {
			if !d.done {
				state := &types.ProgressState{}
				// Create dummy config to pass into processing duplicate check
				active[d.ID] = &types.DownloadConfig{
					URL:      d.URL,
					Filename: d.Filename,
					State:    state,
				}
			}
		}
		return active
	}
	return processing.CheckForDuplicate(url, m.Settings, activeDownloads)
}

// startDownload initiates a new download
func (m RootModel) startDownload(url string, mirrors []string, headers map[string]string, path string, isDefaultPath bool, filename, id string) (RootModel, tea.Cmd) {
	if m.Service == nil {
		m.addLogEntry(LogStyleError.Render("✖ Service unavailable"))
		return m, nil
	}

	// Enforce absolute path
	path = utils.EnsureAbsPath(path)

	candidateFilename := strings.TrimSpace(filename)
	requestID := strings.TrimSpace(id)

	resolvedPath := path
	resolvedFilename := candidateFilename
	optimisticFilename := candidateFilename
	if p, f, err := processing.ResolveDestination(url, candidateFilename, path, isDefaultPath, m.Settings, nil, nil); err == nil {
		resolvedPath = p
		resolvedFilename = f
		if candidateFilename != "" {
			// Only mirror the resolved filename into the optimistic row when the
			// user already chose it; probe-derived names can legitimately change.
			optimisticFilename = f
		}
	} else {
		utils.Debug("Optimistic destination resolve failed for %s: %v", url, err)
	}

	// Call Orchestrator Enqueue
	req := &processing.DownloadRequest{
		URL:                url,
		Filename:           candidateFilename,
		Path:               path,
		Mirrors:            mirrors,
		Headers:            headers,
		IsExplicitCategory: !isDefaultPath,
		SkipApproval:       true,
	}

	optimisticID := requestID
	if optimisticID == "" {
		optimisticID = fmt.Sprintf("pending-%d", time.Now().UnixNano())
	}
	displayName := optimisticFilename
	if displayName == "" {
		displayName = processing.InferFilenameFromURL(url)
	}
	if displayName == "" {
		displayName = "Queued"
	}

	newDownload := NewDownloadModel(optimisticID, url, displayName, 0)
	if resolvedFilename != "" {
		newDownload.Destination = filepath.Join(resolvedPath, resolvedFilename)
	} else {
		newDownload.Destination = resolvedPath
	}
	m.downloads = append(m.downloads, newDownload)
	m.SelectedDownloadID = optimisticID
	m.activeTab = TabQueued
	m.UpdateListItems()

	// Legacy path for tests or startup wiring where processing is not injected yet.
	if m.Orchestrator == nil {
		var (
			newID string
			err   error
		)
		if requestID != "" {
			newID, err = m.Service.AddWithID(
				url,
				resolvedPath,
				resolvedFilename,
				mirrors,
				headers,
				requestID,
				0,
				false,
			)
		} else {
			newID, err = m.Service.Add(
				url,
				resolvedPath,
				resolvedFilename,
				mirrors,
				headers,
				!isDefaultPath,
				0,
				false,
			)
		}
		if err != nil {
			m.removeDownloadByID(optimisticID)
			m.UpdateListItems()
			m.addLogEntry(LogStyleError.Render("✖ Failed to add download: " + err.Error()))
			return m, nil
		}

		if d := m.FindDownloadByID(optimisticID); d != nil {
			d.ID = newID
		}
		if m.SelectedDownloadID == optimisticID {
			m.SelectedDownloadID = newID
		}
		m.UpdateListItems()
		return m, nil
	}

	cmd := func() tea.Msg {
		ctx := m.downloadEnqueueContext()
		var newID string
		var err error
		if requestID != "" {
			newID, err = m.Orchestrator.EnqueueWithID(ctx, req, requestID)
		} else {
			newID, err = m.Orchestrator.Enqueue(ctx, req)
		}
		if err != nil {
			return enqueueErrorMsg{tempID: optimisticID, err: err}
		}
		return enqueueSuccessMsg{
			tempID:   optimisticID,
			id:       newID,
			url:      url,
			path:     resolvedPath,
			filename: optimisticFilename,
		}
	}

	utils.Debug("Queued enqueue command (via Orchestrator): %s -> %s", url, optimisticFilename)
	return m, cmd
}

func (m RootModel) defaultDownloadPath() string {
	if m.Settings != nil {
		if path := strings.TrimSpace(m.Settings.General.DefaultDownloadDir); path != "" {
			return path
		}
	}
	return "."
}

func (m RootModel) downloadEnqueueContext() context.Context {
	if m.enqueueCtx != nil {
		return m.enqueueCtx
	}
	return context.Background()
}

func (m RootModel) isDefaultDownloadPath(path string) bool {
	return utils.EnsureAbsPath(path) == utils.EnsureAbsPath(m.defaultDownloadPath())
}

// Update handles messages and updates the model
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	var cmds []tea.Cmd
	if m.Settings == nil {
		m.Settings = config.DefaultSettings()
	}

	if m.shuttingDown {
		switch msg := msg.(type) {
		case shutdownCompleteMsg:
			if msg.err != nil {
				utils.Debug("TUI shutdown error: %v", msg.err)
			}
			return m, tea.Quit
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			return m, nil
		default:
			return m, nil
		}
	}

	switch msg := msg.(type) {

	case resumeResultMsg:
		if msg.err != nil {
			m.addLogEntry(LogStyleError.Render(fmt.Sprintf("✖ Auto-resume failed for %s: %v", msg.id, msg.err)))
			return m, nil
		}
		if d := m.FindDownloadByID(msg.id); d != nil {
			d.paused = false
			d.pausing = false
			d.resuming = true
		}
		return m, nil

	case enqueueSuccessMsg:
		if msg.tempID != "" && msg.tempID != msg.id {
			temp := m.FindDownloadByID(msg.tempID)
			real := m.FindDownloadByID(msg.id)
			if temp != nil && real != nil && temp != real {
				if real.URL == "" {
					real.URL = temp.URL
				}
				if real.Filename == "" {
					real.Filename = msg.filename
					if real.Filename == "" {
						real.Filename = temp.Filename
					}
					real.FilenameLower = strings.ToLower(real.Filename)
				}
				if real.Destination == "" {
					real.Destination = temp.Destination
				}
				_ = m.removeDownloadByID(msg.tempID)
			} else if temp != nil {
				temp.ID = msg.id
			}
			if m.SelectedDownloadID == msg.tempID {
				m.SelectedDownloadID = msg.id
			}
		}
		m.UpdateListItems()
		return m, nil

	case enqueueErrorMsg:
		if msg.tempID != "" {
			if d := m.FindDownloadByID(msg.tempID); d != nil {
				d.err = msg.err
				d.done = true
				d.paused = false
				d.pausing = false
				d.resuming = false
				d.Speed = 0
				d.Connections = 0
				if d.FilenameLower == "" {
					d.FilenameLower = strings.ToLower(d.Filename)
				}
			} else {
				failed := NewDownloadModel(msg.tempID, "", "", 0)
				failed.err = msg.err
				failed.done = true
				m.downloads = append(m.downloads, failed)
			}
			m.UpdateListItems()
		}
		m.addLogEntry(LogStyleError.Render("✖ Failed to enqueue download: " + msg.err.Error()))
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Calculate list dimensions
		// List goes in bottom-left pane
		availableWidth := msg.Width - 4
		leftWidth := int(float64(availableWidth) * ListWidthRatio)

		// Calculate list height (total height - header row - margins)
		topHeight := 9
		bottomHeight := msg.Height - topHeight - 5
		if bottomHeight < 10 {
			bottomHeight = 10
		}

		m.list.SetSize(leftWidth-2, bottomHeight-4)

		// Update list based on active tab
		m.UpdateListItems()
		return m, nil

	case notificationTickMsg:
		// Notification tick is still used but logs don't expire
		return m, nil

	case UpdateCheckResultMsg:
		if msg.Info != nil && msg.Info.UpdateAvailable {
			m.UpdateInfo = msg.Info
			m.state = UpdateAvailableState
		}
		return m, nil

	case shutdownCompleteMsg:
		if msg.err != nil {
			utils.Debug("TUI shutdown error: %v", msg.err)
		}
		return m, tea.Quit

	case tea.PasteMsg:
		switch m.state {
		case InputState, ExtensionConfirmationState:
			var cmd tea.Cmd
			m.inputs[m.focusedInput], cmd = m.inputs[m.focusedInput].Update(msg)
			return m, cmd
		case URLUpdateState:
			var cmd tea.Cmd
			m.urlUpdateInput, cmd = m.urlUpdateInput.Update(msg)
			return m, cmd
		case SettingsState:
			if m.SettingsIsEditing {
				var cmd tea.Cmd
				m.SettingsInput, cmd = m.SettingsInput.Update(msg)
				return m, cmd
			}
			return m, nil
		case CategoryManagerState:
			if m.catMgrEditing {
				var cmd tea.Cmd
				m.catMgrInputs[m.catMgrEditField], cmd = m.catMgrInputs[m.catMgrEditField].Update(msg)
				return m, cmd
			}
			return m, nil
		default:
			return m, nil
		}

	// Handle filepicker messages for all message types when in FilePickerState
	default:
		if m.state == FilePickerState {
			var cmd tea.Cmd
			m.filepicker, cmd = m.filepicker.Update(msg)

			// Check if a directory was selected
			if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
				return m.handleFilePickerSelection(path)
			}

			return m, cmd
		}

		if m.state == BatchFilePickerState {
			var cmd tea.Cmd
			m.filepicker, cmd = m.filepicker.Update(msg)

			// Check if a file was selected
			if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
				// Read URLs from file
				urls, err := utils.ReadURLsFromFile(path)
				if err != nil {
					m.addLogEntry(LogStyleError.Render("✖ Failed to read batch file: " + err.Error()))
					m.resetFilepickerToDirMode()
					m.state = DashboardState
					return m, nil
				}

				// Store pending URLs and show confirmation
				m.pendingBatchURLs = urls
				m.batchFilePath = path

				// Reset filepicker to directory mode
				m.resetFilepickerToDirMode()

				m.state = BatchConfirmState
				return m, nil
			}

			return m, cmd
		}

	case tea.KeyPressMsg:
		switch m.state {

		case DashboardState:
			return m.updateDashboard(msg)

		case DetailState:
			if msg.String() == "esc" || msg.String() == "q" || msg.String() == "enter" {
				m.state = DashboardState
				return m, nil
			}

		case InputState:
			return m.updateInput(msg)

		case FilePickerState:
			return m.updateFilePicker(msg)

		case HistoryState:
			return m.updateHistory(msg)

		case DuplicateWarningState:
			return m.updateDuplicateWarning(msg)

		case ExtensionConfirmationState:
			return m.updateExtensionConfirmation(msg)

		case BatchFilePickerState:
			return m.updateBatchFilePicker(msg)

		case QuitConfirmState:
			return m.updateQuitConfirm(msg)

		case BatchConfirmState:
			return m.updateBatchConfirm(msg)

		case SettingsState:
			return m.updateSettings(msg)

		case UpdateAvailableState:
			return m.updateUpdateAvailable(msg)

		case URLUpdateState:
			return m.updateURLUpdate(msg)

		case CategoryManagerState:
			return m.updateCategoryManager(msg)

		default:
			if m.state == FilePickerState {

			}
			if m.state == BatchFilePickerState {
				// filepicker non-key messages handled here or move to updateEvents
			}
			return m.updateEvents(msg)
		}
	}

	// Propagate messages to progress bars - only update visible ones for performance
	for _, d := range m.downloads {
		newProgress, cmd := d.progress.Update(msg)
		d.progress = newProgress
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}
