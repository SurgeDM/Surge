//go:build windows

package userservice

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type taskInstallRunner struct {
	commands   []string
	runtimeErr error
	configured bool
	started    bool
}

func (r *taskInstallRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	switch name {
	case "sc.exe":
		return nil, errors.New("legacy service absent")
	case "powershell.exe":
		script := strings.Join(args, " ")
		r.configured = strings.Contains(script, "ExecutionTimeLimit = 'PT0S'") &&
			strings.Contains(script, "DisallowStartIfOnBatteries = $false") &&
			strings.Contains(script, "StopIfGoingOnBatteries = $false")
		return nil, r.runtimeErr
	case "schtasks.exe":
		if args[0] == "/Run" {
			r.started = true
		}
	}
	return nil, nil
}
func TestTaskInstallConfiguresDaemonLifetimeBeforeStarting(t *testing.T) {
	if err := ensureTaskUser(); err != nil {
		t.Skip("requires a non-elevated user")
	}
	for _, fail := range []bool{false, true} {
		runner := &taskInstallRunner{}
		if fail {
			runner.runtimeErr = errors.New("configuration failed")
		}
		manager := &TaskManager{executable: `C:\Program Files\Surge\surge.exe`, taskName: "Surge-S-1-5-21-1000", runner: runner}
		err := manager.Install(context.Background())
		if !runner.configured || runner.started == fail || (err != nil) != fail {
			t.Fatalf("configured=%v started=%v err=%v", runner.configured, runner.started, err)
		}
	}
}

func TestTaskCommandsUseCurrentUserName(t *testing.T) {
	if err := ensureTaskUser(); err != nil {
		t.Skip("requires a non-elevated user")
	}
	manager, err := New(`C:\Surge\surge.exe`)
	if err != nil {
		t.Fatal(err)
	}
	task := manager.(*TaskManager)
	if !strings.HasPrefix(task.taskName, "Surge-S-1-") {
		t.Fatalf("unexpected task name: %s", task.taskName)
	}
	runner := &taskInstallRunner{}
	task.runner = runner
	ctx := context.Background()
	for _, action := range []func(context.Context) error{task.Install, task.Stop, task.Restart, task.Uninstall} {
		if err := action(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := task.Status(ctx); err != nil {
		t.Fatal(err)
	}
	for _, command := range runner.commands {
		if !strings.HasPrefix(command, "sc.exe ") && !strings.Contains(command, task.taskName) {
			t.Fatalf("command does not target current user task: %s", command)
		}
	}
}
