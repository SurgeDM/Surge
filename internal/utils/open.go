package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func OpenFile(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("path is empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return fmt.Errorf("%q is a directory", path)
	}

	return openWithSystem(ctx, path)
}

func OpenContainingFolder(ctx context.Context, path string) error {
	if path == "" {
		return errors.New("path is empty")
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

	return openWithSystem(ctx, targetPath)
}

func openWithSystem(ctx context.Context, path string) error {
	cmd := buildOpenCommand(ctx, path)
	err := cmd.Start()
	if err == nil {
		go func() {
			_ = cmd.Wait()
		}()
	}
	return err
}

func buildOpenCommand(ctx context.Context, path string) *exec.Cmd {
	switch runtime.GOOS {
	case "darwin":
		return exec.CommandContext(ctx, "open", path) //nolint:gosec // internal shell command
	case "windows":
		return exec.CommandContext(ctx, "cmd", "/c", "start", "", path) //nolint:gosec // internal shell command
	default:
		return exec.CommandContext(ctx, "xdg-open", path) //nolint:gosec // internal shell command
	}
}

// OpenBrowser opens a URL in the system's default web browser.
func OpenBrowser(ctx context.Context, url string) error {
	if url == "" {
		return errors.New("url is empty")
	}
	return openWithSystem(ctx, url)
}
