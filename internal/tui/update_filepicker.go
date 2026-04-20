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
		m.state = DashboardState
		return m, nil
	}
	m.pendingBatchURLs = urls
	m.batchFilePath = path
	m.resetFilepickerToDirMode()
	m.state = BatchConfirmState
	return m, nil
}

func (m RootModel) updateFilePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.FilePicker.Cancel) {
		switch m.filepickerOrigin {
		case FilePickerOriginTheme:
			m.Settings.General.ThemePath = m.filepickerOriginalPath
			m.filepickerOrigin = FilePickerOriginNone
			m.state = SettingsState
			m.resetFilepickerToDirMode()
			return m, nil
		case FilePickerOriginSettings:
			m.Settings.General.DefaultDownloadDir = m.filepickerOriginalPath
			m.filepickerOrigin = FilePickerOriginNone
			m.state = SettingsState
			m.resetFilepickerToDirMode()
			return m, nil
		case FilePickerOriginExtension:
			m.inputs[2].SetValue(m.filepickerOriginalPath)
			m.focusInput(2)
			m.filepickerOrigin = FilePickerOriginNone
			m.state = ExtensionConfirmationState
			return m, nil
		case FilePickerOriginCategory:
			m.catMgrInputs[3].SetValue(m.filepickerOriginalPath)
			m.catMgrEditField = 3
			m.blurAllCatInputs()
			m.catMgrInputs[3].Focus()
			m.filepickerOrigin = FilePickerOriginNone
			m.state = CategoryManagerState
			return m, nil
		default:
			m.inputs[2].SetValue(m.filepickerOriginalPath)
			m.focusInput(2)
			m.filepickerOrigin = FilePickerOriginNone
			m.state = InputState
			return m, nil
		}
	}

	// H key to jump to default download directory
	if key.Matches(msg, m.keys.FilePicker.GotoHome) {
		return m, m.handleFilePickerGotoHome()
	}

	// '.' to select current directory — only in directory-picking modes.
	// Skip for FilePickerOriginTheme which is file-only.
	if m.filepickerOrigin != FilePickerOriginTheme && key.Matches(msg, m.keys.FilePicker.UseDir) {
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

func (m RootModel) updateBatchFilePicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.FilePicker.Cancel) {
		// Reset filepicker to directory mode and return
		m.resetFilepickerToDirMode()
		m.state = DashboardState
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
