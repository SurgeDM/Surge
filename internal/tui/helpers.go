package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
	"github.com/SurgeDM/Surge/internal/utils"
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

	// Render each entry at the viewport width so the content fills the pane.
	// TruncateTwoLines ensures long messages don't overflow the UI.

	var wrappedEntries []string
	for _, entry := range m.logEntries {
		wrapped := utils.TruncateTwoLines(entry, width)
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
	switch m.filepickerOrigin {
	case FilePickerOriginTheme:
		m.Settings.General.ThemePath = path
		m.ApplyTheme(m.Settings.General.Theme, path)
		m.filepickerOrigin = FilePickerOriginNone
		m.state = SettingsState
		m.resetFilepickerToDirMode()
		return m, nil
	case FilePickerOriginSettings:
		m.Settings.General.DefaultDownloadDir = path
		m.filepickerOrigin = FilePickerOriginNone
		m.state = SettingsState
		m.resetFilepickerToDirMode()
		return m, nil
	case FilePickerOriginExtension:
		m.inputs[2].SetValue(path)
		m.focusInput(2)
		m.filepickerOrigin = FilePickerOriginNone
		m.state = ExtensionConfirmationState
		return m, nil
	case FilePickerOriginCategory:
		m.catMgrInputs[3].SetValue(path)
		m.catMgrEditField = 3
		m.blurAllCatInputs()
		m.catMgrInputs[3].Focus()
		m.filepickerOrigin = FilePickerOriginNone
		m.state = CategoryManagerState
		return m, nil
	default:
		m.inputs[2].SetValue(path)
		m.focusInput(2)
		m.filepickerOrigin = FilePickerOriginNone
		m.state = InputState
		return m, nil
	}
}

func (m *RootModel) handleFilePickerGotoHome() tea.Cmd {
	var targetDir string
	if m.filepickerOrigin == FilePickerOriginTheme {
		targetDir = config.GetThemesDir()
	} else {
		targetDir = m.Settings.General.DefaultDownloadDir
		if targetDir == "" {
			homeDir, _ := os.UserHomeDir()
			targetDir = filepath.Join(homeDir, "Downloads")
		}
	}
	m.filepicker = newFilepicker(targetDir)
	return m.filepicker.Init()
}

func (m *RootModel) resetFilepickerToDirMode() {
	m.applyFilePickerMode(false, true)
	m.filepicker.AllowedTypes = nil
}

func (m *RootModel) applyFilePickerMode(fileAllowed, dirAllowed bool) {
	m.filepicker.FileAllowed = fileAllowed
	m.filepicker.DirAllowed = dirAllowed

	if fileAllowed {
		m.filepicker.KeyMap.Select = key.NewBinding(key.WithKeys(".", "enter"))
		m.filepicker.KeyMap.Open = key.NewBinding(key.WithKeys(".", "enter", "right"))
	} else {
		m.filepicker.KeyMap.Select = key.NewBinding(key.WithKeys("."))
		m.filepicker.KeyMap.Open = key.NewBinding(key.WithKeys(".", "right"))
	}
}

func (m *RootModel) openDirectoryPicker(origin FilePickerOrigin, originalPath, browseDir string, fileAllowed, dirAllowed bool) tea.Cmd {
	m.filepickerOrigin = origin
	m.filepickerOriginalPath = originalPath
	m.state = FilePickerState
	m.filepicker = newFilepicker(browseDir)
	m.applyFilePickerMode(fileAllowed, dirAllowed)

	return m.filepicker.Init()
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
	return processing.CheckForDuplicate(url, activeDownloads)
}

// renderEmptyMessage provides a consistent visual for "no data" states in dashboard panes.
func renderEmptyMessage(width, height int, message string) string {
	if width < 1 || height < 1 {
		return ""
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		EmptyMessageStyle.Render(message))
}

func (m *RootModel) snapshotSettings() {
	if m.Settings == nil {
		return
	}
	// Shallow copy settings to compare restart-required fields later.
	// Since Settings is a pointer, we clone the underlying struct.
	baseline := *m.Settings
	m.SettingsBaseline = &baseline
}

func (m *RootModel) checkRestartRequirement() bool {
	if m.Settings == nil || m.SettingsBaseline == nil {
		return false
	}

	val1 := reflect.ValueOf(m.Settings).Elem()
	val2 := reflect.ValueOf(m.SettingsBaseline).Elem()
	typ := val1.Type()

	for i := 0; i < typ.NumField(); i++ {
		catField1 := val1.Field(i)
		catField2 := val2.Field(i)
		if catField1.Kind() != reflect.Struct {
			continue
		}

		catTyp := catField1.Type()
		for j := 0; j < catTyp.NumField(); j++ {
			field := catTyp.Field(j)
			if field.Tag.Get("ui_restart") == "true" {
				f1 := catField1.Field(j)
				f2 := catField2.Field(j)
				if !reflect.DeepEqual(f1.Interface(), f2.Interface()) {
					return true
				}
			}
		}
	}
	return false
}
