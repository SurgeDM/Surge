//go:build windows

package userservice

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type taskInstallRunner struct {
	runtimeErr error
	configured bool
	started    bool
}

func (r *taskInstallRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	switch name {
	case "sc.exe":
		return nil, errors.New("legacy service absent")
	case "powershell.exe":
		r.configured = strings.Contains(strings.Join(args, " "), "ExecutionTimeLimit = 'PT0S'")
		return nil, r.runtimeErr
	case "schtasks.exe":
		if args[0] == "/Run" {
			r.started = true
		}
	}
	return nil, nil
}
func TestTaskInstallRemovesRuntimeLimitBeforeStarting(t *testing.T) {
	if err := ensureTaskUser(); err != nil {
		t.Skip("requires a non-elevated user")
	}
	for _, fail := range []bool{false, true} {
		runner := &taskInstallRunner{}
		if fail {
			runner.runtimeErr = errors.New("configuration failed")
		}
		manager := &TaskManager{executable: `C:\Program Files\Surge\surge.exe`, runner: runner}
		err := manager.Install(context.Background())
		if !runner.configured || runner.started == fail || (err != nil) != fail {
			t.Fatalf("configured=%v started=%v err=%v", runner.configured, runner.started, err)
		}
	}
}
