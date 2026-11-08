//go:build windows

package utils

import (
	"fmt"
	"path/filepath"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32   = windows.NewLazySystemDLL("ole32.dll")
	shell32 = windows.NewLazySystemDLL("shell32.dll")
)

// revealFile shows path selected in the user's file manager through
// SHOpenFolderAndSelectItems. Unlike running explorer.exe /select, this goes
// through the shell, so Explorer replacements such as Directory Opus handle it,
// and a window already showing the folder is reused.
func revealFile(path string) error {
	// COM is initialized per OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const coinitApartmentThreaded = 0x2
	if hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, coinitApartmentThreaded); int32(hr) >= 0 {
		defer ole32.NewProc("CoUninitialize").Call()
	}

	// The shell needs backslashes; Clean converts forward slashes.
	p, err := windows.UTF16PtrFromString(filepath.Clean(path))
	if err != nil {
		return err
	}
	pidl, _, _ := shell32.NewProc("ILCreateFromPathW").Call(uintptr(unsafe.Pointer(p)))
	if pidl == 0 {
		return fmt.Errorf("cannot resolve %q in the shell", path)
	}
	defer shell32.NewProc("ILFree").Call(pidl)

	if hr, _, _ := shell32.NewProc("SHOpenFolderAndSelectItems").Call(pidl, 0, 0, 0); int32(hr) < 0 {
		return fmt.Errorf("SHOpenFolderAndSelectItems failed: HRESULT %#x", uint32(hr))
	}
	return nil
}
