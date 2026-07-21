//go:build windows

package cmd

import "golang.org/x/sys/windows"

// isElevated reports whether the current process has administrator
// privileges on Windows by inspecting the process token elevation level.
func isElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
