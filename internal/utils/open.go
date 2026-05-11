package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// OpenFile opens a file in the system's default file explorer or editor.
func OpenFile(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return fmt.Errorf("%q is a directory", path)
	}

	return openWithSystem(path)
}

// OpenContainingFolder opens the containing folder of a file in the system's default file explorer.
func OpenContainingFolder(path string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}

	targetPath := path
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			targetPath = filepath.Dir(path)
		}
	} else {
		targetPath = filepath.Dir(path)
	}

	if targetPath == "" || targetPath == "." {
		return fmt.Errorf("cannot resolve containing folder for %q", path)
	}

	if _, err := os.Stat(targetPath); err != nil {
		return err
	}

	return openWithSystem(targetPath)
}

// openWithSystem opens a path using the system's default command.
func openWithSystem(path string) error {
	cmd := buildOpenCommand(path)
	err := cmd.Start()
	if err == nil {
		go func() {
			_ = cmd.Wait()
		}()
	}
	return err
}

// buildOpenCommand builds the system-specific command for opening a path.
func buildOpenCommand(path string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", path)
	case "windows":
		return exec.Command("cmd", "/c", "start", "", path)
	default:
		return exec.Command("xdg-open", path)
	}
}

// OpenBrowser opens a URL in the system's default web browser.
func OpenBrowser(url string) error {
	if strings.TrimSpace(url) == "" {
		return fmt.Errorf("url is empty")
	}
	return openWithSystem(url)
}

// Run executes an executable with the given arguments and environment.
// On Unix-like systems, it replaces the current process using syscall.Exec.
// On Windows, it starts a new process and exits the current one.
func Run(executable string, args []string, env []string) error {
	return run(executable, args, env)
}
