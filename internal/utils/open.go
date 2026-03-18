package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

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
