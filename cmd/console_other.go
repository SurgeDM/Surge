//go:build !windows

package cmd

// detachConsole is a no-op: service managers on other platforms do not attach
// the server to a visible terminal.
func detachConsole() {}
