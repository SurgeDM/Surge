//go:build !windows

package utils

import (
	"os/exec"
	"runtime"
)

// buildRevealCommand returns a command that shows path selected in the file
// manager, or nil when the platform has no standard way to do so.
func buildRevealCommand(path string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", path)
	}
	return nil
}
