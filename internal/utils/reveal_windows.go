//go:build windows

package utils

import (
	"os/exec"
	"path/filepath"
	"syscall"
)

// buildRevealCommand opens Explorer with path selected. The command line is set
// verbatim because Explorer only understands /select,"path"; the default
// argument quoting would wrap the whole switch and break paths with spaces.
// Explorer also needs backslashes, so the path is cleaned first.
func buildRevealCommand(path string) *exec.Cmd {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + filepath.Clean(path) + `"`}
	return cmd
}
