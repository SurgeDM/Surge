//go:build windows

package userservice

import (
	"context"
	"errors"
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
	checkUser  func() error
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
	if err := m.ensureUser(); err != nil {
		return err
	}
	if _, err := m.runner.Run(ctx, "sc.exe", "query", "surge"); err == nil {
		return fmt.Errorf("%w; remove the legacy Windows service from an elevated terminal before retrying", ErrLegacySystemService)
	}
	// An unscoped ONLOGON trigger requires elevation. Restrict both the
	// trigger and the interactive principal to the current user's SID.
	// Keep the executable separate from its arguments so paths with spaces
	// do not depend on schtasks command-line quoting.
	script := `$ErrorActionPreference = 'Stop'; ` +
		`$user = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value; ` +
		`$action = New-ScheduledTaskAction -Execute '` + strings.ReplaceAll(m.executable, "'", "''") + `' -Argument 'server start --managed-service'; ` +
		`$trigger = New-ScheduledTaskTrigger -AtLogOn -User $user; ` +
		`$principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Limited; ` +
		`Register-ScheduledTask -TaskPath '\' -TaskName '` + m.taskName + `' -Action $action -Trigger $trigger -Principal $principal -Force | Out-Null`
	out, err := m.runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return fmt.Errorf("register user task: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// A daemon must keep running on battery power and beyond the default 72-hour limit.
	out, err = m.runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
		`$ErrorActionPreference = 'Stop'; $task = Get-ScheduledTask -TaskPath '\' -TaskName '`+m.taskName+`'; $task.Settings.ExecutionTimeLimit = 'PT0S'; $task.Settings.DisallowStartIfOnBatteries = $false; $task.Settings.StopIfGoingOnBatteries = $false; Set-ScheduledTask -InputObject $task | Out-Null`)
	if err != nil {
		configureErr := fmt.Errorf("configure user task runtime: %w: %s", err, strings.TrimSpace(string(out)))
		if cleanupErr := m.Uninstall(ctx); cleanupErr != nil {
			return errors.Join(configureErr, fmt.Errorf("remove incompletely configured task: %w", cleanupErr))
		}
		return configureErr
	}
	return m.Start(ctx)
}
func (m *TaskManager) Uninstall(ctx context.Context) error {
	if err := m.ensureUser(); err != nil {
		return err
	}
	state, err := m.stopTask(ctx)
	if err != nil {
		return err
	}
	if state == NotInstalled {
		return nil
	}
	return m.run(ctx, "/Delete", "/F", "/TN", m.taskName)
}
func (m *TaskManager) Start(ctx context.Context) error {
	if err := m.ensureUser(); err != nil {
		return err
	}
	return m.run(ctx, "/Run", "/TN", m.taskName)
}
func (m *TaskManager) Stop(ctx context.Context) error {
	if err := m.ensureUser(); err != nil {
		return err
	}
	_, err := m.stopTask(ctx)
	return err
}
func (m *TaskManager) Restart(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		return err
	}
	return m.Start(ctx)
}

// stopTask accepts a failed /End only when the scheduler confirms no instance
// can be running. Query failures must not permit a restart or unregister a task.
func (m *TaskManager) stopTask(ctx context.Context) (State, error) {
	stopErr := m.run(ctx, "/End", "/TN", m.taskName)
	if stopErr == nil {
		return Stopped, nil
	}
	state, statusErr := m.Status(ctx)
	if statusErr != nil {
		return state, errors.Join(stopErr, statusErr)
	}
	if state == Running {
		return state, stopErr
	}
	return state, nil
}

func (m *TaskManager) Status(ctx context.Context) (State, error) {
	if err := m.ensureUser(); err != nil {
		return NotInstalled, err
	}
	// Numeric TASK_STATE values avoid parsing localized schtasks output.
	script := `$ErrorActionPreference = 'Stop'; try { $task = Get-ScheduledTask -TaskPath '\' -TaskName '` + m.taskName + `'; [int]$task.State } catch { if ($_.CategoryInfo.Category -eq [System.Management.Automation.ErrorCategory]::ObjectNotFound) { -1 } else { throw } }`
	out, err := m.runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	if err != nil {
		return NotInstalled, fmt.Errorf("query user task status: %w: %s", err, strings.TrimSpace(string(out)))
	}
	switch strings.TrimSpace(string(out)) {
	case "-1":
		return NotInstalled, nil
	case "1", "3": // disabled or ready, with no queued/running instances
		return Stopped, nil
	case "2", "4": // queued or running: neither is safe to unregister
		return Running, nil
	default:
		return NotInstalled, fmt.Errorf("query user task status returned unexpected state %q", strings.TrimSpace(string(out)))
	}
}

func (m *TaskManager) ensureUser() error {
	if m.checkUser != nil {
		return m.checkUser()
	}
	return ensureTaskUser()
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
		return fmt.Errorf("schtasks %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
