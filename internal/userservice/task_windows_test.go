//go:build windows

package userservice

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type taskInstallRunner struct {
	commands     []string
	registerErr  error
	runtimeErr   error
	endErr       error
	deleteErr    error
	statusErr    error
	statusOutput string
	deleted      bool
	configured   bool
	started      bool
}

func (r *taskInstallRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, name+" "+strings.Join(args, " "))
	switch name {
	case "sc.exe":
		return nil, errors.New("legacy service absent")
	case "powershell.exe":
		script := strings.Join(args, " ")
		if strings.Contains(script, "Register-ScheduledTask") {
			return nil, r.registerErr
		}
		if !strings.Contains(script, "ExecutionTimeLimit") {
			output := r.statusOutput
			if output == "" {
				output = "3"
			}
			return []byte(output), r.statusErr
		}
		r.configured = strings.Contains(script, "ExecutionTimeLimit = 'PT0S'") &&
			strings.Contains(script, "DisallowStartIfOnBatteries = $false") &&
			strings.Contains(script, "StopIfGoingOnBatteries = $false")
		return nil, r.runtimeErr
	case "schtasks.exe":
		switch args[0] {
		case "/Run":
			r.started = true
		case "/End":
			return nil, r.endErr
		case "/Delete":
			r.deleted = true
			return nil, r.deleteErr
		}
	}
	return nil, nil
}
func TestTaskInstallConfiguresDaemonLifetimeBeforeStarting(t *testing.T) {
	for _, fail := range []bool{false, true} {
		runner := &taskInstallRunner{}
		if fail {
			runner.runtimeErr = errors.New("configuration failed")
		}
		manager := &TaskManager{executable: `C:\Program Files\Surge\surge.exe`, taskName: "Surge-S-1-5-21-1000", runner: runner, checkUser: func() error { return nil }}
		err := manager.Install(context.Background())
		if !runner.configured || runner.started == fail || (err != nil) != fail || runner.deleted != fail {
			t.Fatalf("configured=%v started=%v err=%v", runner.configured, runner.started, err)
		}
	}
}

func TestTaskCommandsUseCurrentUserName(t *testing.T) {
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
	task.checkUser = func() error { return nil }
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

func TestTaskStopFailurePreservesRegistrationAndPreventsRestart(t *testing.T) {
	stopErr := errors.New("stop failed")
	queryErr := errors.New("query failed")
	for _, action := range []string{"uninstall", "restart"} {
		for _, test := range []struct {
			name, output    string
			queryErr        error
			blocked, absent bool
		}{
			{name: "running", output: "4", blocked: true},
			{name: "queued", output: "2", blocked: true},
			{name: "unknown state", output: "0", blocked: true},
			{name: "query failure", queryErr: queryErr, blocked: true},
			{name: "already stopped", output: "3"},
			{name: "disabled", output: "1"},
			{name: "absent", output: "-1", absent: true},
		} {
			t.Run(action+"/"+test.name, func(t *testing.T) {
				runner := &taskInstallRunner{endErr: stopErr, statusOutput: test.output, statusErr: test.queryErr}
				manager := &TaskManager{taskName: "Surge-S-1-5-21-1000", runner: runner, checkUser: func() error { return nil }}
				var err error
				if action == "uninstall" {
					err = manager.Uninstall(context.Background())
				} else {
					err = manager.Restart(context.Background())
				}
				if (err != nil) != test.blocked {
					t.Fatalf("error = %v, want blocked=%v", err, test.blocked)
				}
				if test.blocked {
					if !errors.Is(err, stopErr) || runner.deleted || runner.started {
						t.Fatalf("failed stop lost its error or continued: %v, %+v", err, runner)
					}
					if test.queryErr != nil && !errors.Is(err, test.queryErr) {
						t.Fatalf("lost status error: %v", err)
					}
				} else if action == "uninstall" {
					if runner.deleted == test.absent {
						t.Fatalf("unexpected delete: absent=%v deleted=%v", test.absent, runner.deleted)
					}
				} else if !runner.started {
					t.Fatal("confirmed stopped task was not restarted")
				}
			})
		}
	}
}

func TestTaskInstallReportsRollbackFailure(t *testing.T) {
	configurationErr := errors.New("configuration failed")
	cleanupErr := errors.New("delete failed")
	runner := &taskInstallRunner{runtimeErr: configurationErr, deleteErr: cleanupErr}
	manager := &TaskManager{executable: `C:\Surge\surge.exe`, taskName: "Surge-S-1-5-21-1000", runner: runner, checkUser: func() error { return nil }}
	err := manager.Install(context.Background())
	if !errors.Is(err, configurationErr) || !errors.Is(err, cleanupErr) || !runner.deleted || runner.started {
		t.Fatalf("rollback errors or sequencing incorrect: %v, %+v", err, runner)
	}
}

func TestTaskActionsRejectElevatedSessionBeforeCommands(t *testing.T) {
	runner := &taskInstallRunner{}
	manager := &TaskManager{runner: runner, checkUser: func() error { return ErrRootInstall }}
	for _, action := range []func(context.Context) error{manager.Install, manager.Uninstall, manager.Start, manager.Stop, manager.Restart} {
		if err := action(context.Background()); !errors.Is(err, ErrRootInstall) {
			t.Fatalf("expected elevation rejection, got %v", err)
		}
	}
	if _, err := manager.Status(context.Background()); !errors.Is(err, ErrRootInstall) {
		t.Fatalf("Status did not reject elevated session: %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("elevated session executed commands: %v", runner.commands)
	}
}

func TestTaskInstallRegistersCurrentUserLogon(t *testing.T) {
	for _, fail := range []bool{false, true} {
		runner := &taskInstallRunner{}
		if fail {
			runner.registerErr = errors.New("registration denied")
		}
		manager := &TaskManager{executable: `C:\Users\O'Brien\Surge App\surge.exe`, taskName: "Surge-S-1-5-21-1000", runner: runner, checkUser: func() error { return nil }}
		err := manager.Install(context.Background())
		if !errors.Is(err, runner.registerErr) {
			t.Fatalf("Install error = %v, want %v", err, runner.registerErr)
		}
		if len(runner.commands) < 2 {
			t.Fatalf("missing registration: %v", runner.commands)
		}
		registration := runner.commands[1]
		for _, want := range []string{
			"WindowsIdentity]::GetCurrent().User.Value",
			"New-ScheduledTaskTrigger -AtLogOn -User $user",
			"New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Limited",
			`New-ScheduledTaskAction -Execute 'C:\Users\O''Brien\Surge App\surge.exe' -Argument 'server start --managed-service'`,
			"Register-ScheduledTask -TaskPath '\\' -TaskName 'Surge-S-1-5-21-1000'",
		} {
			if !strings.Contains(registration, want) {
				t.Errorf("registration missing %q: %s", want, registration)
			}
		}
		if fail && (len(runner.commands) != 2 || runner.started || runner.configured || runner.deleted) {
			t.Fatalf("registration failure continued installation: %+v", runner)
		}
	}
}
