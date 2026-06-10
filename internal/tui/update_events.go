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
			m.addLogEntry(LogStyleError.Render(fmt.Sprintf("\u2716 Auto-resume failed for %s: %v", msg.id, msg.err)))
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

				if m.SelectedDownloadID == msg.tempID || (m.GetSelectedDownload() != nil && m.GetSelectedDownload().ID == msg.tempID) {
					m.SelectedDownloadID = msg.id
				}
				_ = m.removeDownloadByID(msg.tempID)
			} else if temp != nil {
				if m.SelectedDownloadID == msg.tempID || (m.GetSelectedDownload() != nil && m.GetSelectedDownload().ID == msg.tempID) {
					m.SelectedDownloadID = msg.id
				}
				temp.ID = msg.id
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
		m.addLogEntry(LogStyleError.Render("\u2716 Failed to enqueue download: " + msg.err.Error()))
		return m, nil

	case events.DownloadRequestMsg:
		return m.handleDownloadRequestMsg(msg, true)

	case events.BatchDownloadRequestMsg:
		return m.handleBatchDownloadRequestMsg(msg, true)

	case events.DownloadStartedMsg:

		found := false
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			d.Filename = msg.Filename
			d.FilenameLower = strings.ToLower(msg.Filename)
			d.Total = msg.Total
			d.Destination = msg.DestPath
			d.RateLimit = msg.RateLimit
			d.RateLimitSet = msg.RateLimitSet
			d.StartTime = time.Now()
			d.paused = false
			d.pausing = false
			// Keep resuming=true for resumed downloads until real transfer starts.
			// Update progress bar
			var progressCmd tea.Cmd
			if d.Total > 0 {
				progressCmd = d.progress.SetPercent(0)
			}
			if d.state == nil && msg.State != nil {
				d.state = msg.State
			}
			if d.state != nil {
				d.state.SetTotalSize(msg.Total) // Keep state updated for verification if needed
			}
			d.started = true
			m.SelectedDownloadID = msg.DownloadID
			m.UpdateListItems()
			m.addLogEntry(LogStyleStarted.Render("\u2b07 Started: " + msg.Filename))
			return m, tea.Batch(progressCmd, m.spinner.Tick)
		}

		if !found {
			newDownload := NewDownloadModel(msg.DownloadID, msg.URL, msg.Filename, msg.Total)
			newDownload.Destination = msg.DestPath
			newDownload.RateLimit = msg.RateLimit
			newDownload.RateLimitSet = msg.RateLimitSet
			if msg.State != nil {
				newDownload.state = msg.State
			}
			newDownload.started = true
			m.downloads = append(m.downloads, newDownload)
			m.SelectedDownloadID = msg.DownloadID
			m.UpdateListItems()
			m.addLogEntry(LogStyleStarted.Render("\u2b07 Started: " + msg.Filename))
			return m, m.spinner.Tick
		}
	case events.ProgressMsg:
		cmd := m.processProgressMsg(msg)
		return m, cmd

	case events.BatchProgressMsg:
		var cmds []tea.Cmd
		for _, bm := range msg {
			cmds = append(cmds, m.processProgressMsg(bm))
		}
		// Only update UI once per batch
		return m, tea.Batch(cmds...)

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
				m.addLogEntry(LogStyleComplete.Render(fmt.Sprintf("\u2714 Done: %s (%.2f MB/s)", d.Filename, speed/float64(config.MB))))
			}
		}
		m.UpdateListItems()
		return m, tea.Batch(cmds...)

	case events.DownloadErrorMsg:
		found := false
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			d.err = msg.Err
			d.done = true
			m.addLogEntry(LogStyleError.Render("\u2716 Error: " + d.Filename))
			found = true
		}
		if !found {
			newDownload := NewDownloadModel(msg.DownloadID, "", msg.Filename, 0)
			newDownload.err = msg.Err
			newDownload.done = true
			m.downloads = append(m.downloads, newDownload)
			m.addLogEntry(LogStyleError.Render("\u2716 Error: " + msg.Filename))
		}
		m.UpdateListItems()
		return m, nil

	case events.DownloadPausedMsg:
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			d.paused = true
			d.pausing = false
			d.resuming = false
			d.Downloaded = msg.Downloaded
			d.RateLimit = msg.RateLimit
			d.RateLimitSet = msg.RateLimitSet
			d.Speed = 0
			m.addLogEntry(LogStylePaused.Render("\u23f8 Paused: " + d.Filename))
		}
		m.UpdateListItems()
		return m, nil

	case events.DownloadResumedMsg:
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			d.paused = false
			d.pausing = false
			d.resuming = true
			m.addLogEntry(LogStyleStarted.Render("\u25b6 Resumed: " + d.Filename))
		}
		m.UpdateListItems()
		return m, m.spinner.Tick

	case events.DownloadQueuedMsg:
		// We optimistically added it, but if it came from elsewhere, handle it
		found := false
		if d := m.FindDownloadByID(msg.DownloadID); d != nil {
			d.RateLimit = msg.RateLimit
			d.RateLimitSet = msg.RateLimitSet
			found = true
		}
		if !found {
			// Add placeholder
			newDownload := NewDownloadModel(msg.DownloadID, msg.URL, msg.Filename, 0)
			newDownload.Destination = msg.DestPath
			newDownload.RateLimit = msg.RateLimit
			newDownload.RateLimitSet = msg.RateLimitSet
			m.downloads = append(m.downloads, newDownload)
			m.SelectedDownloadID = msg.DownloadID
			m.UpdateListItems()
			return m, m.spinner.Tick
		}
		return m, nil

	case events.DownloadRemovedMsg:
		if m.removeDownloadByID(msg.DownloadID) {
			if msg.Filename != "" {
				m.addLogEntry(LogStyleError.Render("\u2716 Removed: " + msg.Filename))
			}
			m.UpdateListItems()
		}
		return m, nil

	case events.SystemLogMsg:
		if msg.Message != "" {
			m.addLogEntry(LogStyleStarted.Render("\u2139 " + msg.Message))
		}
		return m, nil

	case startupConfigWarningMsg:
		for _, w := range msg {
			if w != "" {
				m.addLogEntry(LogStyleError.Render("\u26a0 " + w))
			}
		}
		return m, nil
	}

	return m, nil
}
