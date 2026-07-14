//go:build windows

package power

import (
	"context"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

type osController struct{}

func NewController() Controller {
	return osController{}
}

func (osController) Shutdown(ctx context.Context) error {
	return exec.CommandContext(ctx, "shutdown", "/s", "/t", "0").Run()
}

func (osController) AcquireInhibitor(string) (ReleaseFunc, error) {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	proc := kernel32.NewProc("SetThreadExecutionState")
	ret, _, err := proc.Call(uintptr(esContinuous | esSystemRequired))
	if ret == 0 {
		if err != syscall.Errno(0) {
			return nil, err
		}
		return nil, syscall.EINVAL
	}
	return func() error {
		ret, _, err := proc.Call(uintptr(esContinuous))
		if ret == 0 && err != syscall.Errno(0) {
			return err
		}
		return nil
	}, nil
}
