package tui

import (
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/utils"

	tea "charm.land/bubbletea/v2"
)

func (m RootModel) updatePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {

	if m.state == DashboardState && m.searchActive {
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		m.searchQuery = m.searchInput.Value()
		m.UpdateListItems()
		return m, cmd
	}

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
}

// Update handles messages and updates the model
func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

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

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if m.state == SettingsState {
			m.normalizeSettingsSelection()
			if m.SettingsIsEditing {
				m.updateSettingsInputWidthForViewport()
			}
		}

		if m.state == CategoryManagerState {
			m.normalizeCategoryManagerSelection()
			if m.catMgrEditing {
				m.updateCategoryInputWidthsForViewport()
			}
		}

		// Calculate list dimensions
		// List goes in bottom-left pane — use full width; View() handles the actual box sizing
		availableWidth := msg.Width - 4

		// Calculate list height (total height - header row - margins)
		topHeight := 9
		bottomHeight := msg.Height - topHeight - 5
		if bottomHeight < 10 {
			bottomHeight = 10
		}

		m.list.SetSize(availableWidth-2, bottomHeight-4)

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
		return m.updatePaste(msg)

	// Handle filepicker messages for all message types when in FilePickerState
	default:
		var cmds []tea.Cmd
		for _, d := range m.downloads {
			newProgress, cmd := d.progress.Update(msg)
			d.progress = newProgress
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.state == FilePickerState {
			var cmd tea.Cmd
			m.filepicker, cmd = m.filepicker.Update(msg)
			if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
				model, selCmd := m.handleFilePickerSelection(path)
				return model, tea.Batch(append(cmds, selCmd)...)
			}
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}

		if m.state == BatchFilePickerState {
			var cmd tea.Cmd
			m.filepicker, cmd = m.filepicker.Update(msg)
			if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
				model, selCmd := m.handleBatchFileSelection(path)
				return model, tea.Batch(append(cmds, selCmd)...)
			}
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}
		model, cmd := m.updateEvents(msg)
		cmds = append(cmds, cmd)
		return model, tea.Batch(cmds...)

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

		case HelpModalState:
			if msg.String() == "esc" {
				m.state = DashboardState
				return m, nil
			}
			return m, nil

		default:
			return m, nil
		}
	}

	return m, nil
}
