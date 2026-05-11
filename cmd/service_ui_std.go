//go:build !android

package cmd

import (
	"github.com/SurgeDM/Surge/internal/tui"
	"github.com/kardianos/service"
)

// configureServiceUI wires the TUI settings toggle to kardianos/service
// on platforms with native service manager support.
func configureServiceUI(m *tui.RootModel) {
	s, err := GetService()
	if err != nil {
		return
	}
	status, statusErr := s.Status()
	if statusErr == nil {
		m.Settings.General.AutoStart = (status == service.StatusRunning || status == service.StatusStopped)
	}

	m.ToggleServiceFunc = func(enable bool) error {
		if enable {
			return s.Install()
		}
		_ = s.Stop()
		return s.Uninstall()
	}
}
