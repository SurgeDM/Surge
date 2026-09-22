//go:build linux && !android

package cmd

import (
	"context"

	"github.com/SurgeDM/Surge/internal/tui"
	"github.com/SurgeDM/Surge/internal/userservice"
)

func configureServiceUI(m *tui.RootModel) {
	manager, err := getUserServiceManager()
	if err != nil {
		return
	}
	state, statusErr := manager.Status(context.Background())
	if statusErr == nil {
		m.Settings.General.AutoStart.Value = state != userservice.NotInstalled
	}
	m.ToggleServiceFunc = func(enable bool) error {
		if enable {
			return manager.Install(context.Background())
		}
		return manager.Uninstall(context.Background())
	}
}
