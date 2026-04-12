package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/SurgeDM/Surge/internal/backup"
)

func (m RootModel) exportBundleCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		if m.Transfer == nil {
			return transferExportResultMsg{err: fmt.Errorf("transfer service unavailable")}
		}
		name := fmt.Sprintf("surge-export-%s%s", time.Now().Format("20060102-150405"), backup.BundleExtension)
		target := filepath.Join(dir, name)
		file, err := os.Create(target)
		if err != nil {
			return transferExportResultMsg{err: err}
		}
		defer func() { _ = file.Close() }()
		_, err = m.Transfer.Export(context.Background(), backup.ExportOptions{
			IncludeLogs:     m.transferIncludeLogs,
			IncludePartials: m.transferIncludePartials,
		}, file)
		return transferExportResultMsg{path: target, err: err}
	}
}

func (m RootModel) previewImportCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if m.Transfer == nil {
			return transferPreviewResultMsg{err: fmt.Errorf("transfer service unavailable")}
		}
		file, err := os.Open(filepath.Clean(path))
		if err != nil {
			return transferPreviewResultMsg{err: err}
		}
		defer func() { _ = file.Close() }()
		preview, err := m.Transfer.PreviewImport(context.Background(), file, backup.ImportOptions{
			RootDir: m.transferRootDir,
			Replace: m.transferReplace,
		})
		return transferPreviewResultMsg{preview: preview, err: err}
	}
}

func (m RootModel) applyImportCmd() tea.Cmd {
	return func() tea.Msg {
		if m.Transfer == nil {
			return transferApplyResultMsg{err: fmt.Errorf("transfer service unavailable")}
		}

		var src *os.File
		var err error
		opts := backup.ImportOptions{
			RootDir: m.transferRootDir,
			Replace: m.transferReplace,
		}
		if m.transferPreview != nil {
			opts.SessionID = m.transferPreview.SessionID
		}
		if opts.SessionID == "" {
			src, err = os.Open(filepath.Clean(m.transferImportFile))
			if err != nil {
				return transferApplyResultMsg{err: err}
			}
			defer func() { _ = src.Close() }()
		}
		result, err := m.Transfer.ApplyImport(context.Background(), src, opts)
		return transferApplyResultMsg{result: result, err: err}
	}
}

func (m *RootModel) handleTransferFileSelection(path string) (tea.Model, tea.Cmd) {
	switch m.state {
	case TransferExportPickerState:
		m.state = DataTransferState
		m.transferStatus = "Exporting..."
		return m, m.exportBundleCmd(path)
	case TransferImportPickerState:
		m.transferImportFile = path
		m.state = DataTransferState
		m.transferStatus = "Loading preview..."
		return m, m.previewImportCmd(path)
	case TransferRootPickerState:
		m.transferRootDir = path
		m.transferPreview = nil
		m.transferStatus = "Import root changed. Reload preview."
		m.state = DataTransferState
		return m, nil
	default:
		m.state = DataTransferState
		return m, nil
	}
}

func (m RootModel) updateTransferPicker(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.FilePicker.Cancel) {
		m.state = DataTransferState
		return m, nil
	}

	if key.Matches(msg, m.keys.FilePicker.GotoHome) {
		cmd := m.handleFilePickerGotoHome()
		if m.state == TransferImportPickerState {
			m.filepicker.FileAllowed = true
			m.filepicker.DirAllowed = false
		}
		return m, cmd
	}

	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		return m.handleTransferFileSelection(path)
	}
	return m, cmd
}

func (m RootModel) updateTransfer(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Transfer.Close) {
		m.state = DashboardState
		return m, nil
	}
	if key.Matches(msg, m.keys.Transfer.TogglePartials) {
		m.transferIncludePartials = !m.transferIncludePartials
		return m, nil
	}
	if key.Matches(msg, m.keys.Transfer.ToggleLogs) {
		m.transferIncludeLogs = !m.transferIncludeLogs
		return m, nil
	}
	if key.Matches(msg, m.keys.Transfer.ToggleReplace) {
		m.transferReplace = !m.transferReplace
		m.transferPreview = nil
		m.transferStatus = "Import mode changed. Reload preview."
		return m, nil
	}
	if key.Matches(msg, m.keys.Transfer.BrowseRoot) {
		dir := m.transferRootDir
		if dir == "" {
			dir = m.PWD
		}
		m.filepicker = newFilepicker(dir)
		m.filepicker.DirAllowed = true
		m.filepicker.FileAllowed = false
		m.state = TransferRootPickerState
		return m, m.filepicker.Init()
	}
	if key.Matches(msg, m.keys.Transfer.Export) {
		m.filepicker = newFilepicker(m.PWD)
		m.filepicker.DirAllowed = true
		m.filepicker.FileAllowed = false
		m.state = TransferExportPickerState
		return m, m.filepicker.Init()
	}
	if key.Matches(msg, m.keys.Transfer.Import) {
		m.filepicker = newFilepicker(m.PWD)
		m.filepicker.DirAllowed = false
		m.filepicker.FileAllowed = true
		m.state = TransferImportPickerState
		return m, m.filepicker.Init()
	}
	if key.Matches(msg, m.keys.Transfer.Apply) {
		if m.transferPreview == nil {
			m.transferStatus = "Load an import preview first"
			return m, nil
		}
		m.transferStatus = "Applying import..."
		return m, m.applyImportCmd()
	}
	return m, nil
}
