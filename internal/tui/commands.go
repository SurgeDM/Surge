package tui

import (
	"context"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
	"github.com/SurgeDM/Surge/internal/version"
)

// checkForUpdateCmd performs an async update check
func checkForUpdateCmd(currentVersion string) tea.Cmd {
	return func() tea.Msg {
		info, _ := version.CheckForUpdate(currentVersion)
		return UpdateCheckResultMsg{Info: info}
	}
}

func shutdownCmd(service interface{ Shutdown() error }) tea.Cmd {
	return func() tea.Msg {
		if service == nil {
			return shutdownCompleteMsg{}
		}
		return shutdownCompleteMsg{err: service.Shutdown()}
	}
}

// openWithSystem opens a file or URL with the system's default application
func openWithSystem(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(context.Background(), "open", path)
	case "windows":
		cmd = exec.CommandContext(context.Background(), "cmd", "/c", "start", "", path)
	default: // linux and others
		cmd = exec.CommandContext(context.Background(), "xdg-open", path)
	}
	err := cmd.Start()
	if err == nil {
		go func() {
			_ = cmd.Wait()
		}()
	}
	return err
}
