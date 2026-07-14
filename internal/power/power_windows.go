//go:build windows

package power

import (
	"context"
	"os/exec"
	"runtime"
	"sync"
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

	setExecutionState := func(state uintptr) error {
		ret, _, err := proc.Call(state)
		if ret == 0 {
			if err != syscall.Errno(0) {
				return err
			}
			return syscall.EINVAL
		}
		return nil
	}

	readyCh := make(chan error, 1)
	releaseCh := make(chan struct{})
	doneCh := make(chan struct{})
	var releaseOnce sync.Once
	var releaseErr error

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(doneCh)

		if err := setExecutionState(uintptr(esContinuous | esSystemRequired)); err != nil {
			readyCh <- err
			return
		}
		readyCh <- nil

		<-releaseCh
		releaseErr = setExecutionState(uintptr(esContinuous))
	}()

	if err := <-readyCh; err != nil {
		return nil, err
	}

	return func() error {
		releaseOnce.Do(func() {
			close(releaseCh)
		})
		<-doneCh
		return releaseErr
	}, nil
}
