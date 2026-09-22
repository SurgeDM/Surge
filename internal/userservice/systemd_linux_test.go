//go:build linux && !android

package userservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type recordedCommand struct {
	name string
	args []string
}

type fakeRunner struct {
	commands []recordedCommand
	output   []byte
	err      error
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, recordedCommand{name: name, args: append([]string(nil), args...)})
	return r.output, r.err
}

func TestSystemdUnitRunsManagedServerAsUser(t *testing.T) {
	m := &SystemdManager{executable: "/home/test user/bin/surge"}
	unit := m.unitContents()
	if !strings.Contains(unit, `ExecStart="/home/test user/bin/surge" server start --managed-service`) {
		t.Fatalf("unit has wrong ExecStart:\n%s", unit)
	}
	if strings.Contains(unit, "is-system-service") || strings.Contains(unit, "User=root") {
		t.Fatalf("unit contains system-service identity:\n%s", unit)
	}
}

func TestSystemdInstallWritesAndEnablesUserUnit(t *testing.T) {
	runner := &fakeRunner{}
	m := &SystemdManager{
		executable:  "/opt/surge",
		unitPath:    filepath.Join(t.TempDir(), "surge.service"),
		runner:      runner,
		uid:         1000,
		legacyCheck: func() bool { return false },
	}
	if err := m.Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	data, err := os.ReadFile(m.unitPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "--managed-service") {
		t.Fatal("installed unit does not use managed service mode")
	}
	if len(runner.commands) != 2 {
		t.Fatalf("commands = %v, want daemon-reload and enable", runner.commands)
	}
	want := []string{"--user", "enable", "--now", "surge.service"}
	if strings.Join(runner.commands[1].args, " ") != strings.Join(want, " ") {
		t.Fatalf("enable args = %v, want %v", runner.commands[1].args, want)
	}
}

func TestSystemdInstallRejectsRoot(t *testing.T) {
	m := &SystemdManager{uid: 0}
	if err := m.Install(context.Background()); !errors.Is(err, ErrRootInstall) {
		t.Fatalf("Install error = %v, want ErrRootInstall", err)
	}
}
