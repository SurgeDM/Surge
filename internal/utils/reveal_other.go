//go:build !windows

package utils

import (
	"fmt"
	"os/exec"
	"runtime"
)

// revealFile shows path selected in the file manager. It fails where the
// platform has no standard way to do so, and callers open the folder instead.
// The command is waited on so a failed reveal also reaches that fallback;
// `open -R` returns as soon as Finder has the request.
func revealFile(path string) error {
	cmd := buildRevealCommand(path)
	if cmd == nil {
		return fmt.Errorf("revealing a file is not supported on %s", runtime.GOOS)
	}
	return cmd.Run()
}

// buildRevealCommand returns a command that shows path selected in the file
// manager, or nil when the platform has no standard way to do so.
func buildRevealCommand(path string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", path)
	}
	return nil
}
