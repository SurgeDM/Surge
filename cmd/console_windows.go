//go:build windows

package cmd

import "golang.org/x/sys/windows"

// detachConsole releases the console that Windows creates when the scheduled
// task launches the server, so the service leaves no terminal window behind.
func detachConsole() {
	// The result is ignored: there is no useful fallback, and logging is not set up yet.
	_, _, _ = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole").Call()
}
