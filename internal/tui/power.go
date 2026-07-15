package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/power"
	"github.com/SurgeDM/Surge/internal/utils"
)

const (
	autoShutdownReason     = "Surge is waiting for downloads to finish before shutting down."
	autoShutdownRetryDelay = 30 * time.Second
)

func (m RootModel) isAutoShutdownEnabled() bool {
	return m.Settings != nil && config.Resolve[bool](m.Settings.General.AutoShutdownAfterDownloads)
}

func (m RootModel) hasPendingDownloads() bool {
	for _, d := range m.downloads {
		if d == nil {
			continue
		}
		if d.done || d.err != nil {
			continue
		}
		return true
	}
	return false
}

func (m *RootModel) ensurePowerController() {
	if m.powerController == nil {
		m.powerController = power.NewController()
	}
}

func (m *RootModel) releasePowerInhibitor() {
	if m.powerInhibitorRelease == nil {
		return
	}
	if err := m.powerInhibitorRelease(); err != nil {
		utils.Debug("Auto-shutdown: failed to release power inhibitor: %v", err)
	}
	m.powerInhibitorRelease = nil
}

func (m *RootModel) acquirePowerInhibitor() {
	m.ensurePowerController()
	if m.powerInhibitorRelease != nil {
		return
	}
	release, err := m.powerController.AcquireInhibitor(autoShutdownReason)
	if err != nil {
		m.addLogEntry(LogStyleError.Render("\u26a0 Auto-shutdown enabled, but sleep prevention failed: " + err.Error()))
		utils.Debug("Auto-shutdown: acquire inhibitor failed: %v", err)
		return
	}
	m.powerInhibitorRelease = release
}

func (m RootModel) refreshAutoShutdown() (RootModel, tea.Cmd) {
	if !m.isAutoShutdownEnabled() {
		m.autoShutdownArmed = false
		m.autoShutdownTriggered = false
		m.autoShutdownRetrying = false
		m.releasePowerInhibitor()
		return m, nil
	}

	if m.autoShutdownTriggered || m.autoShutdownRetrying {
		return m, nil
	}

	pending := m.hasPendingDownloads()
	if !m.autoShutdownArmed {
		if !pending {
			return m, nil
		}
		m.autoShutdownArmed = true
		m.acquirePowerInhibitor()
		m.addLogEntry(LogStyleStarted.Render("\u23fb Auto-shutdown armed"))
		return m, nil
	}

	if pending {
		return m, nil
	}

	m.autoShutdownTriggered = true
	m.releasePowerInhibitor()
	m.ensurePowerController()
	m.addLogEntry(LogStyleStarted.Render("\u23fb All downloads finished; shutting down computer"))

	controller := m.powerController
	return m, func() tea.Msg {
		return autoShutdownResultMsg{err: controller.Shutdown(context.Background())}
	}
}

func (m *RootModel) applyAutoShutdownSettingChange() {
	if !m.isAutoShutdownEnabled() {
		m.autoShutdownArmed = false
		m.autoShutdownTriggered = false
		m.autoShutdownRetrying = false
		m.releasePowerInhibitor()
		return
	}

	if m.autoShutdownTriggered || m.autoShutdownRetrying || m.autoShutdownArmed || !m.hasPendingDownloads() {
		return
	}

	m.autoShutdownArmed = true
	m.acquirePowerInhibitor()
	m.addLogEntry(LogStyleStarted.Render("\u23fb Auto-shutdown armed"))
}

func (m RootModel) handleAutoShutdownResult(err error) (RootModel, tea.Cmd) {
	if err != nil {
		m.autoShutdownTriggered = false
		m.autoShutdownRetrying = true
		m.acquirePowerInhibitor()
		m.addLogEntry(LogStyleError.Render(fmt.Sprintf("\u2716 Auto-shutdown failed: %v", err)))
		return m, tea.Tick(autoShutdownRetryDelay, func(time.Time) tea.Msg {
			return autoShutdownRetryMsg{}
		})
	}
	return m, nil
}

func (m RootModel) handleAutoShutdownRetry() (RootModel, tea.Cmd) {
	m.autoShutdownRetrying = false
	return m.refreshAutoShutdown()
}
