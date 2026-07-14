//go:build darwin

package power

import (
	"context"
	"os/exec"
)

type osController struct{}

func NewController() Controller {
	return osController{}
}

func (osController) Shutdown(ctx context.Context) error {
	if err := exec.CommandContext(ctx, "osascript", "-e", `tell application "System Events" to shut down`).Run(); err == nil {
		return nil
	}
	return exec.CommandContext(ctx, "shutdown", "-h", "now").Run()
}

func (osController) AcquireInhibitor(reason string) (ReleaseFunc, error) {
	cmd := exec.Command("caffeinate", "-dimsu")
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := cmd.Process.Kill(); err != nil {
			return err
		}
		_, _ = cmd.Process.Wait()
		return nil
	}, nil
}
