package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/bugreport"
	"github.com/SurgeDM/Surge/internal/clipboard"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/utils"
)

var openBugReportBrowser = utils.OpenBrowser
var writeBugReportClipboard = clipboard.Write

func (m RootModel) updateDuplicateWarning(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Duplicate.Continue) {
		// Continue anyway - startDownload handles unique filename generation
		m.state = DashboardState
		return m.startDownload(m.pendingURL, m.pendingMirrors, m.pendingHeaders, m.pendingPath, m.pendingIsDefaultPath, m.pendingFilename, "")
	}
	if key.Matches(msg, m.keys.Duplicate.Cancel) {
		// Cancel - don't add
		m.state = DashboardState
		return m, nil
	}
	if key.Matches(msg, m.keys.Duplicate.Focus) {
		// Focus existing download - find it and select in list
		for i, d := range m.getFilteredDownloads() {
			if d.URL == m.pendingURL {
				m.list.Select(i)
				break
			}
		}
		m.state = DashboardState
		return m, nil
	}
	return m, nil
}

func (m RootModel) updateQuitConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {

	confirmQuit := func() (tea.Model, tea.Cmd) {
		if m.cancelEnqueue != nil {
			m.cancelEnqueue()
		}
		m.shuttingDown = true
		return m, shutdownCmd(m.Service)
	}
	cancelQuit := func() (tea.Model, tea.Cmd) {
		m.state = DashboardState
		m.quitConfirmFocused = 0
		return m, nil
	}
	if key.Matches(msg, m.keys.QuitConfirm.Left) || key.Matches(msg, m.keys.QuitConfirm.Right) {
		m.quitConfirmFocused = 1 - m.quitConfirmFocused
		return m, nil
	}
	if key.Matches(msg, m.keys.QuitConfirm.Yes) {
		return confirmQuit()
	}
	if key.Matches(msg, m.keys.QuitConfirm.No) {
		return cancelQuit()
	}
	if key.Matches(msg, m.keys.QuitConfirm.Select) {
		if m.quitConfirmFocused == 0 {
			return confirmQuit()
		}
		return cancelQuit()
	}
	if key.Matches(msg, m.keys.QuitConfirm.Cancel) {
		return cancelQuit()
	}
	return m, nil
}

func (m RootModel) updateBatchConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {

	if key.Matches(msg, m.keys.BatchConfirm.Confirm) {
		// Add all URLs as downloads, skipping duplicates
		path := m.Settings.General.DefaultDownloadDir
		if path == "" {
			path = "."
		}

		added := 0
		skipped := 0
		var batchCmds []tea.Cmd
		for _, url := range m.pendingBatchURLs {
			// Skip duplicate URLs
			if m.checkForDuplicate(url) != nil {
				skipped++
				continue
			}
			var cmd tea.Cmd
			m, cmd = m.startDownload(url, nil, nil, path, true, "", "")
			if cmd != nil {
				batchCmds = append(batchCmds, cmd)
			}
			added++
		}

		if skipped > 0 {
			m.addLogEntry(LogStyleStarted.Render(fmt.Sprintf("\u2b07 Added %d downloads from batch (%d duplicates skipped)", added, skipped)))
		} else {
			m.addLogEntry(LogStyleStarted.Render(fmt.Sprintf("\u2b07 Added %d downloads from batch", added)))
		}
		m.pendingBatchURLs = nil
		m.batchFilePath = ""
		m.state = DashboardState
		return m, tea.Batch(batchCmds...)
	}
	if key.Matches(msg, m.keys.BatchConfirm.Cancel) {
		m.pendingBatchURLs = nil
		m.batchFilePath = ""
		m.state = DashboardState
		return m, nil
	}
	return m, nil
}

func (m RootModel) updateURLUpdate(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {

	if key.Matches(msg, m.keys.Input.Esc) {
		m.state = DashboardState
		m.urlUpdateInput.SetValue("")
		m.urlUpdateInput.Blur()
		return m, nil
	}
	if key.Matches(msg, m.keys.Input.Enter) {
		newURL := strings.TrimSpace(m.urlUpdateInput.Value())
		if newURL != "" {
			if d := m.GetSelectedDownload(); d != nil {
				if err := m.Service.UpdateURL(d.ID, newURL); err != nil {
					m.addLogEntry(LogStyleError.Render(fmt.Sprintf("\u2716 Failed to update URL: %s", err.Error())))
				} else {
					m.addLogEntry(LogStyleComplete.Render(fmt.Sprintf("\u2714 URL Updated: %s", d.Filename)))
					d.URL = newURL
				}
			}
		}
		m.state = DashboardState
		m.urlUpdateInput.SetValue("")
		m.urlUpdateInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.urlUpdateInput, cmd = m.urlUpdateInput.Update(msg)
	return m, cmd
}

func (m RootModel) updateUpdateAvailable(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Update.OpenGitHub) {
		// Open the release page in browser
		if m.UpdateInfo != nil && m.UpdateInfo.ReleaseURL != "" {
			_ = openWithSystem(m.UpdateInfo.ReleaseURL)
		}
		m.state = DashboardState
		m.UpdateInfo = nil
		return m, nil
	}
	if key.Matches(msg, m.keys.Update.IgnoreNow) {
		// Just dismiss the modal
		m.state = DashboardState
		m.UpdateInfo = nil
		return m, nil
	}
	if key.Matches(msg, m.keys.Update.NeverRemind) {
		// Persist the setting and dismiss
		m.Settings.General.SkipUpdateCheck = true
		_ = m.persistSettings()
		m.state = DashboardState
		m.UpdateInfo = nil
		return m, nil
	}

	return m, nil
}

func (m RootModel) updateBugReportTarget(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.BugReport.Cancel) {
		m = m.resetBugReportFlow()
		return m, nil
	}

	if key.Matches(msg, m.keys.BugReport.Core) {
		m.bugReportIncludeSystemInfo = true
		m.bugReportIncludeLatestLog = true
		m.quitConfirmFocused = 0
		m.state = BugReportSystemDetailsState
		return m, nil
	}

	if key.Matches(msg, m.keys.BugReport.Extension) {
		reportURL := bugreport.ExtensionBugReportURL()
		m = m.tryOpenBugReportURL(reportURL)
		m = m.resetBugReportFlow()
		return m, nil
	}

	return m, nil
}

func (m RootModel) updateBugReportSystemDetails(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m, decision, handled := m.handleYesNoSelection(msg)
	if !handled {
		return m, nil
	}

	switch decision {
	case yesNoNone:
		return m, nil
	case yesNoCancel:
		m = m.resetBugReportFlow()
	case yesNoYes:
		m.bugReportIncludeSystemInfo = true
		m.quitConfirmFocused = 0
		m.state = BugReportLogPathState
	case yesNoNo:
		m.bugReportIncludeSystemInfo = false
		m.quitConfirmFocused = 0
		m.state = BugReportLogPathState
	}

	return m, nil
}

func (m RootModel) updateBugReportLogPath(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m, decision, handled := m.handleYesNoSelection(msg)
	if !handled {
		return m, nil
	}

	switch decision {
	case yesNoNone:
		return m, nil
	case yesNoCancel:
		m = m.resetBugReportFlow()
	case yesNoYes:
		m.bugReportIncludeLatestLog = true
		reportURL := m.buildCoreBugReportURL()
		m = m.tryOpenBugReportURL(reportURL)
		m = m.resetBugReportFlow()
	case yesNoNo:
		m.bugReportIncludeLatestLog = false
		reportURL := m.buildCoreBugReportURL()
		m = m.tryOpenBugReportURL(reportURL)
		m = m.resetBugReportFlow()
	}

	return m, nil
}

type yesNoDecision int

const (
	yesNoNone yesNoDecision = iota
	yesNoCancel
	yesNoYes
	yesNoNo
)

func (m RootModel) handleYesNoSelection(msg tea.KeyPressMsg) (RootModel, yesNoDecision, bool) {
	if key.Matches(msg, m.keys.QuitConfirm.Left) || key.Matches(msg, m.keys.QuitConfirm.Right) {
		m.quitConfirmFocused = 1 - m.quitConfirmFocused
		return m, yesNoNone, true
	}

	if key.Matches(msg, m.keys.QuitConfirm.Yes) {
		return m, yesNoYes, true
	}

	if key.Matches(msg, m.keys.QuitConfirm.No) {
		return m, yesNoNo, true
	}

	if key.Matches(msg, m.keys.QuitConfirm.Select) {
		if m.quitConfirmFocused == 0 {
			return m, yesNoYes, true
		}
		return m, yesNoNo, true
	}

	if key.Matches(msg, m.keys.QuitConfirm.Cancel) {
		return m, yesNoCancel, true
	}

	return m, yesNoNone, false
}

func (m RootModel) buildCoreBugReportURL() string {
	return bugreport.CoreBugReportURL(bugreport.CoreReportOptions{
		Version:              m.CurrentVersion,
		Commit:               m.CurrentCommit,
		IncludeSystemDetails: m.bugReportIncludeSystemInfo,
		IncludeLatestLogPath: m.bugReportIncludeLatestLog,
	})
}

func (m RootModel) tryOpenBugReportURL(reportURL string) RootModel {
	if reportURL == "" {
		m.addLogEntry(LogStyleError.Render("✖ Could not open browser. Try running surge bug-report from your terminal instead."))
		return m
	}

	if err := openBugReportBrowser(reportURL); err != nil {
		if err := writeBugReportClipboard(reportURL); err == nil {
			m.addLogEntry(LogStyleError.Render("✖ Could not open browser. URL copied to clipboard."))
			return m
		}

		m.addLogEntry(LogStyleError.Render("✖ Could not open browser. Try running surge bug-report from your terminal instead."))
		return m
	}

	m.addLogEntry(LogStyleStarted.Render("🐞 Opening browser to file bug report..."))
	return m
}

func (m RootModel) resetBugReportFlow() RootModel {
	m.bugReportIncludeSystemInfo = true
	m.bugReportIncludeLatestLog = true
	m.quitConfirmFocused = 0
	m.state = DashboardState
	return m
}

func (m RootModel) updateRestartConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	confirmRestart := func() (tea.Model, tea.Cmd) {
		if m.cancelEnqueue != nil {
			m.cancelEnqueue()
		}
		m.shuttingDown = true
		m.RestartRequested = true
		return m, shutdownCmd(m.Service)
	}
	cancelRestart := func() (tea.Model, tea.Cmd) {
		m.state = DashboardState
		m.quitConfirmFocused = 0
		m.SettingsBaseline = nil
		return m, nil
	}

	m, decision, handled := m.handleYesNoSelection(msg)
	if !handled {
		return m, nil
	}

	switch decision {
	case yesNoYes:
		return confirmRestart()
	case yesNoNo, yesNoCancel:
		return cancelRestart()
	}

	return m, nil
}

func (m RootModel) updateCategoryResetConfirm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	confirmReset := func() (tea.Model, tea.Cmd) {
		defaults := config.DefaultSettings()
		m.Settings.Categories = defaults.Categories
		m.addLogEntry(LogStyleStarted.Render("\u2714 Categories reset to defaults"))
		utils.Debug("Categories Reset to Defaults")
		m.state = SettingsState
		m.quitConfirmFocused = 0
		return m, nil
	}
	cancelReset := func() (tea.Model, tea.Cmd) {
		m.state = SettingsState
		m.quitConfirmFocused = 0
		return m, nil
	}

	m, decision, handled := m.handleYesNoSelection(msg)
	if !handled {
		return m, nil
	}

	switch decision {
	case yesNoYes:
		return confirmReset()
	case yesNoNo, yesNoCancel:
		return cancelReset()
	}

	return m, nil
}
