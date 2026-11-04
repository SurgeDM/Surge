//go:build windows

package cmd

import "golang.org/x/sys/windows"

// detachConsole releases the console that Windows creates when the scheduled
// task launches the server, so the service leaves no terminal window behind.
func detachConsole() {
	_, _, _ = windows.NewLazySystemDLL("kernel32.dll").NewProc("FreeConsole").Call()
}
