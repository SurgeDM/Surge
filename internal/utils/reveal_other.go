//go:build !windows

package utils

import (
	"fmt"
	"os/exec"
	"runtime"
)

// revealFile shows path selected in the file manager. It fails where the
// platform has no standard way to do so, and callers open the folder instead.
func revealFile(path string) error {
	cmd := buildRevealCommand(path)
	if cmd == nil {
		return fmt.Errorf("revealing a file is not supported on %s", runtime.GOOS)
	}
	return startCommand(cmd)
}

// buildRevealCommand returns a command that shows path selected in the file
// manager, or nil when the platform has no standard way to do so.
func buildRevealCommand(path string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", path)
	}
	return nil
}
