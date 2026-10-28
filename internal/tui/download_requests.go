package tui

import (
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
)

func (m RootModel) handleDownloadRequestMsg(msg types.DownloadEvent, queueIfBusy bool) (tea.Model, tea.Cmd) {
	if msg.ApprovalResult != nil {
		if msg.ApprovalContext != nil && msg.ApprovalContext.Err() != nil {
			return m, nil
		}
		id := strings.TrimSpace(msg.DownloadID)
		duplicate := id != "" && m.FindDownloadByID(id) != nil
		if id != "" && (m.state == ExtensionConfirmationState || m.state == DuplicateWarningState) && m.pendingDownloadID == id {
			duplicate = true
		}
		for _, pending := range m.pendingRequestQueue {
			if id != "" && strings.TrimSpace(pending.DownloadID) == id {
				duplicate = true
			}
		}
		for _, pending := range m.pendingBatchRequests {
			if id != "" && strings.TrimSpace(pending.DownloadID) == id && m.state == BatchConfirmState {
				duplicate = true
			}
		}
		for _, batch := range m.pendingBatchRequestQueue {
			for _, pending := range batch.BatchEvents {
				if id != "" && strings.TrimSpace(pending.DownloadID) == id {
					duplicate = true
				}
			}
		}
		if duplicate {
			if msg.ApprovalContext == nil {
				msg.ApprovalResult <- errors.New("download ID already exists")
			} else {
				select {
				case msg.ApprovalResult <- errors.New("download ID already exists"):
				case <-msg.ApprovalContext.Done():
				}
			}
			return m, nil
		}
		if msg.ApprovalContext == nil {
			msg.ApprovalResult <- nil
		} else {
			select {
			case msg.ApprovalResult <- nil:
			case <-msg.ApprovalContext.Done():
				return m, nil
			}
		}
		msg.ApprovalResult = nil
	}
	msg.DownloadID = strings.TrimSpace(msg.DownloadID)
	if msg.DownloadID != "" {
		duplicateID := m.FindDownloadByID(msg.DownloadID) != nil ||
			(m.isRequestConfirmationBusy() && m.pendingDownloadID == msg.DownloadID)
		for _, queued := range m.pendingRequestQueue {
			duplicateID = duplicateID || strings.TrimSpace(queued.DownloadID) == msg.DownloadID
		}
		if duplicateID {
			m.addStyledLogEntry(logError, "\u2716 Download ID already exists")
			if !queueIfBusy {
				return m.showNextPendingRequest()
			}
			return m, nil
		}
	}

	if queueIfBusy && m.isRequestConfirmationBusy() {
		m.pendingRequestQueue = append(m.pendingRequestQueue, msg)
		return m, nil
	}

	m.pendingDownloadID = msg.DownloadID
	path := strings.TrimSpace(msg.Path)
	isDefaultPath := m.isDefaultDownloadPath(path)
	if path == "" {
		isDefaultPath = true
		path = m.defaultDownloadPath()
	}
	if msg.IsExplicitCategory {
		isDefaultPath = false
	}

	duplicate := m.checkForDuplicate(msg.URL)
	m.pendingSkipDuplicateWarning = msg.SkipDuplicateWarning

	if duplicate != nil && config.Resolve[bool](m.Settings.General.WarnOnDuplicate) && !msg.SkipDuplicateWarning {
		utils.Debug("Duplicate download detected in TUI for host %s", utils.URLHostForLog(msg.URL))
		m.pendingDownloadID = msg.DownloadID
		m.pendingURL = msg.URL
		m.pendingMirrors = msg.Mirrors
		m.pendingHeaders = msg.Headers
		m.pendingPath = path
		m.pendingIsDefaultPath = isDefaultPath
		m.pendingFilename = msg.Filename
		m.pendingDownloadID = msg.DownloadID
		m.pendingWorkers = msg.Workers
		m.pendingMinChunkSize = msg.MinChunkSize
		m.duplicateInfo = duplicate.Filename
		m.state = DuplicateWarningState
		return m, nil
	}

	if m.Settings != nil && config.Resolve[bool](m.Settings.Extension.ExtensionPrompt) {
		m.pendingDownloadID = msg.DownloadID
		m.pendingURL = msg.URL
		m.pendingMirrors = msg.Mirrors
		m.pendingHeaders = msg.Headers
		m.pendingPath = path
		m.pendingIsDefaultPath = isDefaultPath
		m.pendingFilename = msg.Filename
		m.pendingDownloadID = msg.DownloadID
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
	m.pendingDownloadID = ""
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
