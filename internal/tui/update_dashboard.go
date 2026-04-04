package tui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/clipboard"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/engine/types"
)

func (m RootModel) updateDashboard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {

	// Handle search input FIRST when active (intercepts ALL keys)
	if m.searchActive {
		switch msg.String() {
		case "esc":
			// Cancel search and clear query
			m.searchActive = false
			m.searchInput.Blur()
			m.searchQuery = ""
			m.searchInput.SetValue("")
			m.UpdateListItems()
			return m, nil
		case "enter":
			// Commit search (keep filter applied)
			m.searchActive = false
			m.searchInput.Blur()
			return m, nil
		default:
			// All other keys go to search input
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.searchQuery = m.searchInput.Value()
			m.UpdateListItems()
			return m, cmd
		}
	}

	// Toggle search with F
	if key.Matches(msg, m.keys.Dashboard.Search) {
		if m.searchQuery != "" {
			// Clear existing search
			m.searchQuery = ""
			m.searchInput.SetValue("")
			m.UpdateListItems()
		} else {
			// Start new search
			m.searchActive = true
			m.searchInput.Focus()
		}
		return m, nil
	}

	// Tab switching
	switchTab := func(tab int) (tea.Model, tea.Cmd) {
		m.activeTab = tab
		m.ManualTabSwitch = true
		m.UpdateListItems()
		return m, nil
	}

	if key.Matches(msg, m.keys.Dashboard.TabQueued) {
		return switchTab(TabQueued)
	}
	if key.Matches(msg, m.keys.Dashboard.TabActive) {
		return switchTab(TabActive)
	}
	if key.Matches(msg, m.keys.Dashboard.TabDone) {
		return switchTab(TabDone)
	}
	// Quit
	if key.Matches(msg, m.keys.Dashboard.Quit, m.keys.Dashboard.ForceQuit) {
		m.state = QuitConfirmState
		m.quitConfirmFocused = 0
		return m, nil
	}

	// Add download
	if key.Matches(msg, m.keys.Dashboard.Add) {
		m.state = InputState
		m.focusedInput = 0
		m.inputs[0].Focus()
		// Use default download dir from settings
		defaultDir := m.Settings.General.DefaultDownloadDir
		if defaultDir == "" {
			defaultDir = "."
		}
		m.inputs[2].SetValue(defaultDir)
		m.inputs[2].Blur()
		m.inputs[3].SetValue("")
		m.inputs[3].Blur()
		m.inputs[1].SetValue("") // Clear mirrors
		m.inputs[1].Blur()

		url := ""
		if m.Settings.General.ClipboardMonitor {
			url = clipboard.ReadURL()
		}
		m.inputs[0].SetValue(url)
		return m, nil
	}

	// Next Tab
	if key.Matches(msg, m.keys.Dashboard.NextTab) {
		m.activeTab = (m.activeTab + 1) % 3
		m.ManualTabSwitch = true
		m.UpdateListItems()
		return m, nil
	}

	// Delete download
	if key.Matches(msg, m.keys.Dashboard.Delete) {
		if m.list.FilterState() == list.Filtering {
			// Fall through
		} else if d := m.GetSelectedDownload(); d != nil {
			if m.Service == nil {
				m.addLogEntry(LogStyleError.Render("✖ Service unavailable"))
				return m, nil
			}
			targetID := d.ID

			// Call Service Delete
			if err := m.Service.Delete(targetID); err != nil {
				m.addLogEntry(LogStyleError.Render("✖ Delete failed: " + err.Error()))
			} else {
				m.removeDownloadByID(targetID)
			}
			m.UpdateListItems()
			return m, nil
		}
	}

	// Pause/Resume toggle
	if key.Matches(msg, m.keys.Dashboard.Pause) {
		if d := m.GetSelectedDownload(); d != nil {
			if m.Service == nil {
				m.addLogEntry(LogStyleError.Render("✖ Service unavailable"))
				return m, nil
			}
			if !d.done {
				if d.paused {
					// Resume
					d.paused = false
					d.resuming = true
					if err := m.Service.Resume(d.ID); err != nil {
						m.addLogEntry(LogStyleError.Render("✖ Resume failed: " + err.Error()))
						d.paused = true // Revert
						d.resuming = false
					}
				} else {
					// Pause
					if err := m.Service.Pause(d.ID); err != nil {
						m.addLogEntry(LogStyleError.Render("✖ Pause failed: " + err.Error()))
					} else {
						d.resuming = false
						d.pausing = true
					}
				}
			}
		}
		m.UpdateListItems()
		return m, nil
	}

	// Open file
	if key.Matches(msg, m.keys.Dashboard.OpenFile) {
		if d := m.GetSelectedDownload(); d != nil {
			canOpen := d.done || (m.Settings.Network.SequentialDownload && !d.paused && d.Downloaded > 0)
			if canOpen && d.Destination != "" {
				filePath := d.Destination
				if !d.done {
					filePath = d.Destination + types.IncompleteSuffix
				}
				_ = openWithSystem(filePath)
			}
		}
		return m, nil
	}

	// Refresh URL
	if key.Matches(msg, m.keys.Dashboard.Refresh) {
		if d := m.GetSelectedDownload(); d != nil {
			if m.Service == nil {
				m.addLogEntry(LogStyleError.Render("✖ Service unavailable"))
				return m, nil
			}
			// Only allow refresh if download is paused or errored
			if d.paused || d.err != nil {
				m.state = URLUpdateState
				m.urlUpdateInput.SetValue(d.URL)
				m.urlUpdateInput.Focus()
			} else {
				m.addLogEntry(LogStyleError.Render("✖ Pause download before refreshing URL"))
			}
		}
		return m, nil
	}

	// Other keys...
	if key.Matches(msg, m.keys.Dashboard.Log) {
		m.logFocused = !m.logFocused
		return m, nil
	}

	if key.Matches(msg, m.keys.Dashboard.ToggleHelp) {
		m.state = HelpModalState
		return m, nil
	}

	if key.Matches(msg, m.keys.Dashboard.Settings) {
		m.state = SettingsState
		m.SettingsActiveTab = 0
		m.SettingsSelectedRow = 0
		m.SettingsIsEditing = false
		return m, nil
	}

	if key.Matches(msg, m.keys.Dashboard.CategoryFilter) {
		if !m.Settings.General.CategoryEnabled || len(m.Settings.General.Categories) == 0 {
			if m.categoryFilter != "" {
				m.categoryFilter = ""
				m.addLogEntry(LogStyleStarted.Render("📂 Filter: All"))
				m.UpdateListItems()
				return m, nil
			}
			m.addLogEntry(LogStyleError.Render("✖ Enable categories in Settings first"))
			return m, nil
		}
		names := config.CategoryNames(m.Settings.General.Categories)
		cycle := append([]string{""}, names...)
		cycle = append(cycle, "Uncategorized")
		current := 0
		for i, n := range cycle {
			if n == m.categoryFilter {
				current = i
				break
			}
		}
		m.categoryFilter = cycle[(current+1)%len(cycle)]
		label := m.categoryFilter
		if label == "" {
			label = "All"
		}
		m.addLogEntry(LogStyleStarted.Render("📂 Filter: " + label))
		m.UpdateListItems()
		return m, nil
	}

	if key.Matches(msg, m.keys.Dashboard.BatchImport) {
		m.state = BatchFilePickerState
		m.filepicker = newFilepicker(m.PWD)
		m.filepicker.FileAllowed = true
		m.filepicker.DirAllowed = false
		return m, m.filepicker.Init()
	}

	if m.logFocused {
		if key.Matches(msg, m.keys.Dashboard.LogClose) {
			m.logFocused = false
			return m, nil
		}
		if key.Matches(msg, m.keys.Dashboard.LogDown) {
			m.logViewport.ScrollDown(1)
			return m, nil
		}
		if key.Matches(msg, m.keys.Dashboard.LogUp) {
			m.logViewport.ScrollUp(1)
			return m, nil
		}
		if key.Matches(msg, m.keys.Dashboard.LogTop) {
			m.logViewport.GotoTop()
			return m, nil
		}
		if key.Matches(msg, m.keys.Dashboard.LogBottom) {
			m.logViewport.GotoBottom()
			return m, nil
		}
		return m, nil
	}

	// Block bare ESC from propagating (only quit via ctrl+q/ctrl+c)
	if msg.String() == "esc" {
		return m, nil
	}

	// Pass messages to the list for navigation/filtering
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
