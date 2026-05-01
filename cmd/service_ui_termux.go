//go:build android

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SurgeDM/Surge/internal/tui"
)

// configureServiceUI wires the TUI settings toggle to Termux's runit/sv
// service manager on Android.
func configureServiceUI(m *tui.RootModel) {
	if !isTermuxServicesAvailable() {
		return
	}
	svcDir := termuxServiceDir()
	if info, err := os.Stat(svcDir); err == nil && info.IsDir() {
		_, downErr := os.Stat(filepath.Join(svcDir, "down"))
		m.Settings.General.AutoStart = os.IsNotExist(downErr)
	}

	m.ToggleServiceFunc = func(enable bool) error {
		if enable {
			return installTermuxService()
		}
		_ = stopTermuxService()
		return uninstallTermuxService()
	}
}

func installTermuxService() error {
	if !isTermuxServicesAvailable() {
		return fmt.Errorf("termux-services is not available. Install it with: pkg install termux-services")
	}
	svcDir := termuxServiceDir()
	if _, err := os.Stat(svcDir); err == nil {
		return fmt.Errorf("service already installed at %s", svcDir)
	}
	if err := os.MkdirAll(svcDir, 0o755); err != nil {
		return fmt.Errorf("failed to create service directory %s: %w", svcDir, err)
	}
	// Write run script
	runPath := filepath.Join(svcDir, "run")
	if err := writeRunScript(runPath, termuxServiceRunScript()); err != nil {
		_ = os.RemoveAll(svcDir)
		return fmt.Errorf("failed to write run script: %w", err)
	}
	// Set up log service per termux-services convention:
	// mkdir <svc>/log, symlink <svc>/log/run -> $PREFIX/share/termux-services/svlogger
	logDir := filepath.Join(svcDir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		_ = os.RemoveAll(svcDir)
		return fmt.Errorf("failed to create log directory: %w", err)
	}
	svloggerPath := filepath.Join(defaultPrefix(), "share", "termux-services", "svlogger")
	logRunPath := filepath.Join(logDir, "run")
	if err := os.Symlink(svloggerPath, logRunPath); err != nil {
		// Fallback: write a script if symlink fails (e.g. across filesystems)
		logScript := "#!/" + defaultPrefix() + "/bin/sh\nexec svlogd -tt " + filepath.Join(defaultPrefix(), "var", "log", "sv", "surge") + "\n"
		if err2 := writeRunScript(logRunPath, logScript); err2 != nil {
			_ = os.RemoveAll(svcDir)
			return fmt.Errorf("failed to set up log service: %w", err)
		}
	}
	// Create down file so service doesn't auto-start until explicitly started
	_ = os.WriteFile(filepath.Join(svcDir, "down"), nil, 0o644)
	return nil
}

func stopTermuxService() error {
	_, _ = sv("down", svServiceName())
	return nil
}

func uninstallTermuxService() error {
	svcDir := termuxServiceDir()
	_ = stopTermuxService()
	return os.RemoveAll(svcDir)
}
