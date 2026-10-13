//go:build windows

package userservice

import (
	"context"
	"fmt"
	"os/exec"
	"os/user"
	"strings"

	"golang.org/x/sys/windows"
)

type taskRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type taskExecRunner struct{}

func (taskExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type TaskManager struct {
	executable string
	taskName   string
	runner     taskRunner
}

func New(executable string) (Manager, error) {
	if strings.TrimSpace(executable) == "" {
		return nil, fmt.Errorf("service executable path is empty")
	}
	current, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("resolve service user: %w", err)
	}
	return &TaskManager{executable: executable, taskName: "Surge-" + current.Uid, runner: taskExecRunner{}}, nil
}

func (m *TaskManager) Install(ctx context.Context) error {
	if err := ensureTaskUser(); err != nil {
		return err
	}
	if _, err := m.runner.Run(ctx, "sc.exe", "query", "surge"); err == nil {
		return fmt.Errorf("%w; remove the legacy Windows service from an elevated terminal before retrying", ErrLegacySystemService)
	}
	command := `\"` + m.executable + `\" server start --managed-service`
	if err := m.run(ctx, "/Create", "/F", "/SC", "ONLOGON", "/RL", "LIMITED", "/TN", m.taskName, "/TR", command); err != nil {
		return err
	}
	// Preserve the task's other settings while removing the default 72-hour limit.
	out, err := m.runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`$ErrorActionPreference = 'Stop'; $task = Get-ScheduledTask -TaskPath '\' -TaskName '`+m.taskName+`'; $task.Settings.ExecutionTimeLimit = 'PT0S'; Set-ScheduledTask -InputObject $task | Out-Null`)
	if err != nil {
		return fmt.Errorf("configure user task runtime: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return m.Start(ctx)
}
func (m *TaskManager) Uninstall(ctx context.Context) error {
	if err := ensureTaskUser(); err != nil {
		return err
	}
	_, _ = m.runner.Run(ctx, "schtasks.exe", "/End", "/TN", m.taskName)
	return m.run(ctx, "/Delete", "/F", "/TN", m.taskName)
}
func (m *TaskManager) Start(ctx context.Context) error {
	if err := ensureTaskUser(); err != nil {
		return err
	}
	return m.run(ctx, "/Run", "/TN", m.taskName)
}
func (m *TaskManager) Stop(ctx context.Context) error {
	if err := ensureTaskUser(); err != nil {
		return err
	}
	return m.run(ctx, "/End", "/TN", m.taskName)
}
func (m *TaskManager) Restart(ctx context.Context) error {
	if err := ensureTaskUser(); err != nil {
		return err
	}
	_, _ = m.runner.Run(ctx, "schtasks.exe", "/End", "/TN", m.taskName)
	return m.Start(ctx)
}
func (m *TaskManager) Status(ctx context.Context) (State, error) {
	if err := ensureTaskUser(); err != nil {
		return NotInstalled, err
	}
	out, err := m.runner.Run(ctx, "schtasks.exe", "/Query", "/TN", m.taskName, "/FO", "CSV", "/NH")
	if err != nil {
		return NotInstalled, nil
	}
	if strings.Contains(strings.ToLower(string(out)), "running") {
		return Running, nil
	}
	return Stopped, nil
}
func ensureTaskUser() error {
	if windows.GetCurrentProcessToken().IsElevated() {
		return ErrRootInstall
	}
	return nil
}
func (m *TaskManager) run(ctx context.Context, args ...string) error {
	out, err := m.runner.Run(ctx, "schtasks.exe", args...)
	if err != nil {
		return fmt.Errorf("schtasks %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}
