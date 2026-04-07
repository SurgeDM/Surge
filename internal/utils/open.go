package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

const (
	ChromeExtensionURL  = "https://github.com/SurgeDM/Surge/releases/latest"
	FirefoxExtensionURL = "https://addons.mozilla.org/en-US/firefox/addon/surge/"
)

// OpenBrowser opens a URL in the user's default browser
func OpenBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// WriteToClipboard copies text to the system clipboard using platform-specific tools
func WriteToClipboard(text string) {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
	case "windows":
		cmd := exec.Command("clip")
		cmd.Stdin = strings.NewReader(text)
		_ = cmd.Run()
	default:
		// Try xclip first, fall back to wl-clipboard
		cmd := exec.Command("xclip", "-selection", "clipboard")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			cmd = exec.Command("wl-copy")
			cmd.Stdin = strings.NewReader(text)
			_ = cmd.Run()
		}
	}
}
