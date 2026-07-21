//go:build !windows

package cmd

import "os"

// isElevated reports whether the current process is running with root
// privileges. Used to select the correct token file path: root processes
// (system service daemons) use GetSystemStateDir, others use GetStateDir.
func isElevated() bool {
	return os.Getuid() == 0
}
