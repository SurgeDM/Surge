package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/utils"
)

func (m *RootModel) handleBatchFileSelection(path string) (tea.Model, tea.Cmd) {
	urls, err := utils.ReadURLsFromFile(path)
	if err != nil {
		m.addLogEntry(LogStyleError.Render("✖ Failed to read batch file: " + err.Error()))
		m.resetFilepickerToDirMode()
		m.uiState = DashboardState
		return m, nil
	}
	m.pendingBatchURLs = urls
	m.batchFilePath = path
	m.resetFilepickerToDirMode()
	m.uiState = BatchConfirmState
	return m, nil
}

func (m *RootModel) updateFilePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.FilePicker.Cancel) {
		// Cancel and return to appropriate state
		if m.SettingsFileBrowsing {
			m.Settings.General.DefaultDownloadDir = m.filepickerOriginalPath
			m.SettingsFileBrowsing = false
			m.uiState = SettingsState
			return m, nil
		}
		if m.ExtensionFileBrowsing {
			m.inputs[2].SetValue(m.filepickerOriginalPath)
			m.ExtensionFileBrowsing = false
			m.uiState = ExtensionConfirmationState
			return m, nil
		}
		if m.catMgrFileBrowsing {
			m.catMgrInputs[3].SetValue(m.filepickerOriginalPath)
			m.catMgrFileBrowsing = false
			m.uiState = CategoryManagerState
			return m, nil
		}
		m.inputs[2].SetValue(m.filepickerOriginalPath)
		m.uiState = InputState
		return m, nil
	}

	// H key to jump to default download directory
	if key.Matches(msg, m.keys.FilePicker.GotoHome) {
		cmd := m.handleFilePickerGotoHome()
		return m, cmd
	}

	// '.' to select current directory
	if key.Matches(msg, m.keys.FilePicker.UseDir) {
		return m.handleFilePickerSelection(m.filepicker.CurrentDirectory)
	}

	// Pass key to filepicker
	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)

	// Check if a directory was selected
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		return m.handleFilePickerSelection(path)
	}

	return m, cmd
}

func (m *RootModel) updateBatchFilePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.FilePicker.Cancel) {
		// Reset filepicker to directory mode and return
		m.resetFilepickerToDirMode()
		m.uiState = DashboardState
		return m, nil
	}

	// H key to jump to default download directory
	if key.Matches(msg, m.keys.FilePicker.GotoHome) {
		cmd := m.handleFilePickerGotoHome()
		m.filepicker.FileAllowed = true
		m.filepicker.DirAllowed = false
		return m, cmd
	}

	// Pass key to filepicker
	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)

	// Check if a file was selected
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		return m.handleBatchFileSelection(path)
	}

	return m, cmd
}
