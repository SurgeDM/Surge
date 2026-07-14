//go:build linux

package power

import (
	"context"
	"errors"
	"os/exec"
)

type osController struct{}

func NewController() Controller {
	return osController{}
}

func (osController) Shutdown(ctx context.Context) error {
	if err := exec.CommandContext(ctx, "systemctl", "poweroff").Run(); err == nil {
		return nil
	}
	return exec.CommandContext(ctx, "shutdown", "-h", "now").Run()
}

func (osController) AcquireInhibitor(reason string) (ReleaseFunc, error) {
	if _, err := exec.LookPath("systemd-inhibit"); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return func() error { return nil }, nil
		}
		return nil, err
	}

	cmd := exec.Command(
		"systemd-inhibit",
		"--what=sleep:shutdown",
		"--mode=block",
		"--why="+reason,
		"sleep",
		"infinity",
	)
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
