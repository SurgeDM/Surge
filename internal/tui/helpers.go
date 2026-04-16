package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
)

// addLogEntry adds a log entry to the log viewport
func (m *RootModel) addLogEntry(msg string) {
	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", timestamp, msg)
	m.logEntries = append(m.logEntries, entry)

	// Keep only the last 100 entries to prevent memory issues
	if len(m.logEntries) > 100 {
		m.logEntries = m.logEntries[len(m.logEntries)-100:]
	}

	// Re-wrap and update viewport content
	m.refreshLogViewportContent()
	// Auto-scroll to bottom
	m.logViewport.GotoBottom()
}

// refreshLogViewportContent re-renders log entries with current viewport wrapping.
func (m *RootModel) refreshLogViewportContent() {
	width := m.logViewport.Width()
	if width <= 0 {
		return
	}

	var wrappedEntries []string
	for _, entry := range m.logEntries {
		// Wrap each entry to the current viewport width
		wrapped := lipgloss.NewStyle().Width(width).Render(entry)
		wrappedEntries = append(wrappedEntries, strings.Split(wrapped, "\n")...)
	}

	// Bottom-align entries if they don't fill the viewport
	height := m.logViewport.Height()
	if height > 0 && len(wrappedEntries) < height {
		padding := make([]string, height-len(wrappedEntries))
		wrappedEntries = append(padding, wrappedEntries...)
	}

	m.logViewport.SetContent(strings.Join(wrappedEntries, "\n"))
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
