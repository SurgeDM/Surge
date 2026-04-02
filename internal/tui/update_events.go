package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/events"
	"github.com/SurgeDM/Surge/internal/tui/components"
	"github.com/SurgeDM/Surge/internal/utils"
)

func (m RootModel) updateEvents(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg := msg.(type) {

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)

		needsSpinner := false
		for _, d := range m.downloads {
			if d.pausing || d.resuming || components.DetermineStatus(d.done, d.paused, d.err != nil, d.Speed, d.Downloaded) == components.StatusQueued {
				needsSpinner = true
				break
			}
		}
		if needsSpinner {
			m.UpdateListItems()
			return m, cmd
		}
		return m, nil

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
		return m, m.spinner.Tick

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

	case events.DownloadRequestMsg:
		path := strings.TrimSpace(msg.Path)
		isDefaultPath := m.isDefaultDownloadPath(path)
		if path == "" {
			isDefaultPath = true
			path = m.defaultDownloadPath()
		}

		duplicate := m.checkForDuplicate(msg.URL)

		if duplicate != nil && m.Settings.General.WarnOnDuplicate {
			utils.Debug("Duplicate download detected in TUI: %s", msg.URL)
			m.pendingURL = msg.URL
			m.pendingMirrors = msg.Mirrors
			m.pendingHeaders = msg.Headers
			m.pendingPath = path
			m.pendingIsDefaultPath = isDefaultPath
			m.pendingFilename = msg.Filename
			m.duplicateInfo = duplicate.Filename
			m.state = DuplicateWarningState
			return m, nil
		}

		if m.Settings.General.ExtensionPrompt {
			m.pendingURL = msg.URL
			m.pendingMirrors = msg.Mirrors
			m.pendingHeaders = msg.Headers
			m.pendingPath = path
			m.pendingIsDefaultPath = isDefaultPath
			m.pendingFilename = msg.Filename
			m.inputs[2].SetValue(path)
			m.inputs[3].SetValue(msg.Filename)
			m.focusedInput = 2
			for i := range m.inputs {
				m.inputs[i].Blur()
			}
			m.inputs[m.focusedInput].Focus()
			m.state = ExtensionConfirmationState
			return m, nil
		}

		return m.startDownload(msg.URL, msg.Mirrors, msg.Headers, path, isDefaultPath, msg.Filename, msg.ID)

	case events.DownloadStartedMsg:

		found := false
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			d.Filename = msg.Filename
			d.FilenameLower = strings.ToLower(msg.Filename)
			d.Total = msg.Total
			d.Destination = msg.DestPath
			d.StartTime = time.Now()
			d.paused = false
			d.pausing = false
			// Keep resuming=true for resumed downloads until real transfer starts.
			// Update progress bar
			if d.Total > 0 {
				d.progress.SetPercent(0)
			}
			if d.state == nil && msg.State != nil {
				d.state = msg.State
			}
			if d.state != nil {
				d.state.SetTotalSize(msg.Total) // Keep state updated for verification if needed
			}
			found = true
		}

		if !found {
			newDownload := NewDownloadModel(msg.DownloadID, msg.URL, msg.Filename, msg.Total)
			newDownload.Destination = msg.DestPath
			if msg.State != nil {
				newDownload.state = msg.State
			}
			m.downloads = append(m.downloads, newDownload)
		}

		m.UpdateListItems()
		m.addLogEntry(LogStyleStarted.Render("⬇ Started: " + msg.Filename))
		return m, m.spinner.Tick

	case events.ProgressMsg:
		m.processProgressMsg(msg)
		return m, nil

	case events.BatchProgressMsg:
		for _, msg := range msg {
			m.processProgressMsg(msg)
		}
		// Only update UI once per batch
		return m, nil

	case events.DownloadCompleteMsg:

		var cmds []tea.Cmd

		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			if !d.done {
				d.Total = msg.Total
				d.Downloaded = d.Total
				d.Elapsed = msg.Elapsed
				d.Speed = msg.AvgSpeed
				d.done = true
				cmds = append(cmds, d.progress.SetPercent(1.0))

				speed := d.Speed
				if msg.Elapsed.Seconds() >= 1 {
					speed = float64(d.Total) / float64(int(msg.Elapsed.Seconds()))
				} else if msg.Elapsed.Seconds() > 0 {
					speed = float64(d.Total) / msg.Elapsed.Seconds()
				}
				m.addLogEntry(LogStyleComplete.Render(fmt.Sprintf("✔ Done: %s (%.2f MB/s)", d.Filename, speed/float64(config.MB))))
			}
		}
		m.UpdateListItems()
		return m, tea.Batch(cmds...)

	case events.DownloadErrorMsg:
		found := false
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			d.err = msg.Err
			d.done = true
			m.addLogEntry(LogStyleError.Render("✖ Error: " + d.Filename))
			found = true
		}
		if !found {
			newDownload := NewDownloadModel(msg.DownloadID, "", msg.Filename, 0)
			newDownload.err = msg.Err
			newDownload.done = true
			m.downloads = append(m.downloads, newDownload)
			m.addLogEntry(LogStyleError.Render("✖ Error: " + msg.Filename))
		}
		m.UpdateListItems()
		return m, nil

	case events.DownloadPausedMsg:
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			d.paused = true
			d.pausing = false
			d.resuming = false
			d.Downloaded = msg.Downloaded
			d.Speed = 0
			m.addLogEntry(LogStylePaused.Render("⏸ Paused: " + d.Filename))
		}
		m.UpdateListItems()
		return m, nil

	case events.DownloadResumedMsg:
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			d.paused = false
			d.pausing = false
			d.resuming = true
			m.addLogEntry(LogStyleStarted.Render("▶ Resumed: " + d.Filename))
		}
		m.UpdateListItems()
		return m, m.spinner.Tick

	case events.DownloadQueuedMsg:
		// We optimistically added it, but if it came from elsewhere, handle it
		found := false
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			found = true
		}
		if !found {
			// Add placeholder
			newDownload := NewDownloadModel(msg.DownloadID, msg.URL, msg.Filename, 0)
			newDownload.Destination = msg.DestPath
			m.downloads = append(m.downloads, newDownload)
			m.UpdateListItems()
			return m, m.spinner.Tick
		}
		return m, nil

	case events.DownloadRemovedMsg:
		if m.removeDownloadByID(msg.DownloadID) {
			if msg.Filename != "" {
				m.addLogEntry(LogStyleError.Render("✖ Removed: " + msg.Filename))
			}
			m.UpdateListItems()
		}
		return m, nil

	case events.SystemLogMsg:
		if msg.Message != "" {
			m.addLogEntry(LogStyleStarted.Render("ℹ " + msg.Message))
		}
		return m, nil
	}

	return m, nil
}
