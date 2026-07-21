package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

func (m RootModel) handleDownloadRequestMsg(msg types.DownloadEvent, queueIfBusy bool) (tea.Model, tea.Cmd) {
	if queueIfBusy && m.isRequestConfirmationBusy() {
		m.pendingRequestQueue = append(m.pendingRequestQueue, msg)
		return m, nil
	}

	path := strings.TrimSpace(msg.Path)
	isDefaultPath := m.isDefaultDownloadPath(path)
	if path == "" {
		isDefaultPath = true
		path = m.defaultDownloadPath()
	}

	duplicate := m.checkForDuplicate(msg.URL)

	if duplicate != nil && config.Resolve[bool](m.Settings.General.WarnOnDuplicate) {
		utils.Debug("Duplicate download detected in TUI: %s", msg.URL)
		m.pendingURL = msg.URL
		m.pendingMirrors = msg.Mirrors
		m.pendingHeaders = msg.Headers
		m.pendingPath = path
		m.pendingIsDefaultPath = isDefaultPath
		m.pendingFilename = msg.Filename
		m.pendingWorkers = msg.Workers
		m.pendingMinChunkSize = msg.MinChunkSize
		m.duplicateInfo = duplicate.Filename
		m.state = DuplicateWarningState
		return m, nil
	}

	if m.Settings != nil && config.Resolve[bool](m.Settings.Extension.ExtensionPrompt) {
		m.pendingURL = msg.URL
		m.pendingMirrors = msg.Mirrors
		m.pendingHeaders = msg.Headers
		m.pendingPath = path
		m.pendingIsDefaultPath = isDefaultPath
		m.pendingFilename = msg.Filename
		m.pendingWorkers = msg.Workers
		m.pendingMinChunkSize = msg.MinChunkSize
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

	return m.startDownload(msg.URL, msg.Mirrors, msg.Headers, path, isDefaultPath, msg.Filename, msg.DownloadID, msg.Workers, msg.MinChunkSize)
}

func (m RootModel) handleBatchDownloadRequestMsg(msg types.DownloadEvent, queueIfBusy bool) (tea.Model, tea.Cmd) {
	if queueIfBusy && m.isRequestConfirmationBusy() {
		m.pendingBatchRequestQueue = append(m.pendingBatchRequestQueue, msg)
		return m, nil
	}

	m.pendingBatchURLs = nil
	m.pendingBatchRequests = append([]types.DownloadEvent(nil), msg.BatchEvents...)
	m.batchFilePath = strings.TrimSpace(msg.Path)
	if m.batchFilePath == "" {
		m.batchFilePath = m.defaultDownloadPath()
	}
	m.inputs[2].SetValue(m.batchFilePath)
	m.state = BatchConfirmState
	return m, nil
}

func (m RootModel) isRequestConfirmationBusy() bool {
	return m.state == ExtensionConfirmationState ||
		m.state == DuplicateWarningState ||
		m.state == BatchConfirmState ||
		m.state == RemoveConfirmState
}

func (m RootModel) showNextPendingRequest() (tea.Model, tea.Cmd) {
	if len(m.pendingRequestQueue) == 0 {
		if len(m.pendingBatchRequestQueue) == 0 {
			return m, nil
		}
		next := m.pendingBatchRequestQueue[0]
		m.pendingBatchRequestQueue = m.pendingBatchRequestQueue[1:]
		return m.handleBatchDownloadRequestMsg(next, false)
	}
	next := m.pendingRequestQueue[0]
	m.pendingRequestQueue = m.pendingRequestQueue[1:]
	return m.handleDownloadRequestMsg(next, false)
}
