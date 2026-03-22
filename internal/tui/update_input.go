package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/surge-downloader/surge/internal/utils"
)

func (m *RootModel) focusInput(i int) {
	m.inputs[m.focusedInput].Blur()
	m.focusedInput = i
	m.inputs[m.focusedInput].Focus()
}

func (m *RootModel) blurAllInputs() {
	for i := range m.inputs {
		m.inputs[i].Blur()
	}
}

func (m RootModel) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Input.Esc) {
		m.state = DashboardState
		return m, nil
	}

	if key.Matches(msg, m.keys.Input.Tab) && m.focusedInput == 2 {
		m.state = FilePickerState
		m.filepicker = newFilepicker(m.PWD)
		return m, m.filepicker.Init()
	}

	if key.Matches(msg, m.keys.Input.Up) && m.focusedInput > 0 {
		m.focusInput(m.focusedInput - 1)
		return m, nil
	}

	if key.Matches(msg, m.keys.Input.Down) && m.focusedInput < 3 {
		m.focusInput(m.focusedInput + 1)
		return m, nil
	}

	if key.Matches(msg, m.keys.Input.Enter) {
		if m.focusedInput < 3 {
			m.focusInput(m.focusedInput + 1)
			return m, nil
		}
		return m.submitInputForm()
	}

	var cmd tea.Cmd
	m.inputs[m.focusedInput], cmd = m.inputs[m.focusedInput].Update(msg)
	return m, cmd
}

func (m RootModel) submitInputForm() (tea.Model, tea.Cmd) {
	inputVal := m.inputs[0].Value()
	if inputVal == "" {
		m.blurAllInputs()
		m.focusedInput = 0
		m.inputs[0].Focus()
		return m, nil
	}

	url, mirrors := parseURLInput(inputVal)

	// Append mirrors from dedicated mirror input
	if mirrorsVal := m.inputs[1].Value(); mirrorsVal != "" {
		for _, part := range strings.Split(mirrorsVal, ",") {
			if cleaned := strings.TrimSpace(part); cleaned != "" {
				mirrors = append(mirrors, cleaned)
			}
		}
	}

	if url == "" {
		m.focusedInput = 0
		m.inputs[0].Focus()
		return m, nil
	}

	pathInput := strings.TrimSpace(m.inputs[2].Value())
	path := pathInput
	isDefaultPath := m.isDefaultDownloadPath(path)
	if path == "" {
		isDefaultPath = true
		path = m.defaultDownloadPath()
	}
	filename := m.inputs[3].Value()

	if d := m.checkForDuplicate(url); d != nil {
		m.pendingURL = url
		m.pendingMirrors = mirrors
		m.pendingHeaders = nil
		m.pendingPath = path
		m.pendingIsDefaultPath = isDefaultPath
		m.pendingFilename = filename
		m.duplicateInfo = d.Filename
		m.state = DuplicateWarningState
		return m, nil
	}

	m.state = DashboardState
	m.inputs[0].SetValue("")
	m.inputs[1].SetValue("")
	m.inputs[2].SetValue(path) // Keep path for next download
	m.inputs[3].SetValue("")

	return m.startDownload(url, mirrors, nil, path, isDefaultPath, filename, "")
}

// parseURLInput splits a comma-separated URL string into a primary URL and mirrors.
func parseURLInput(input string) (url string, mirrors []string) {
	for _, part := range strings.Split(input, ",") {
		cleaned := strings.TrimSpace(part)
		if cleaned == "" {
			continue
		}
		if url == "" {
			url = cleaned
		} else {
			mirrors = append(mirrors, cleaned)
		}
	}
	return
}

func (m RootModel) updateExtensionConfirmation(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Extension.Browse) && m.focusedInput == 2 {
		m.ExtensionFileBrowsing = true
		browseDir := strings.TrimSpace(m.inputs[2].Value())
		if browseDir == "" {
			browseDir = m.PWD
		}
		m.state = FilePickerState
		m.filepicker = newFilepicker(browseDir)
		return m, m.filepicker.Init()
	}

	if key.Matches(msg, m.keys.Extension.Next) || key.Matches(msg, m.keys.Extension.Prev) {
		m.blurAllInputs()
		if m.focusedInput == 2 {
			m.focusedInput = 3
		} else {
			m.focusedInput = 2
		}
		m.inputs[m.focusedInput].Focus()
		return m, nil
	}

	if key.Matches(msg, m.keys.Extension.Confirm) {
		m.pendingPath = strings.TrimSpace(m.inputs[2].Value())
		m.pendingFilename = strings.TrimSpace(m.inputs[3].Value())
		m.pendingIsDefaultPath = m.isDefaultDownloadPath(m.pendingPath)
		if m.pendingPath == "" {
			m.pendingIsDefaultPath = true
			m.pendingPath = m.defaultDownloadPath()
		}

		if d := m.checkForDuplicate(m.pendingURL); d != nil {
			utils.Debug("Duplicate download detected after confirmation: %s", m.pendingURL)
			m.duplicateInfo = d.Filename
			m.state = DuplicateWarningState
			return m, nil
		}

		m.state = DashboardState
		return m.startDownload(m.pendingURL, m.pendingMirrors, m.pendingHeaders, m.pendingPath, m.pendingIsDefaultPath, m.pendingFilename, "")
	}

	if key.Matches(msg, m.keys.Extension.Cancel) {
		m.ExtensionFileBrowsing = false
		m.blurAllInputs()
		m.state = DashboardState
		return m, nil
	}

	var cmd tea.Cmd
	m.inputs[m.focusedInput], cmd = m.inputs[m.focusedInput].Update(msg)
	return m, cmd
}
