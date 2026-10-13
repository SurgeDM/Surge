//go:build linux && !android

package userservice

import (
	"context"
	"errors"
	"os"
	"os/exec"
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
	want := []string{"--user", "enable", "--now", m.unitPath}
	if strings.Join(runner.commands[1].args, " ") != strings.Join(want, " ") {
		t.Fatalf("enable args = %v, want %v", runner.commands[1].args, want)
	}
}

func TestSystemdInstallRejectsRoot(t *testing.T) {
	m := &SystemdManager{uid: 0}
	if err := m.Install(context.Background()); !errors.Is(err, ErrRootInstall) {
		t.Fatalf("Install error = %v, want ErrRootInstall", err)
	}
	if _, err := m.Status(context.Background()); !errors.Is(err, ErrRootInstall) {
		t.Fatalf("Status error = %v, want ErrRootInstall", err)
	}
	if err := m.Start(context.Background()); !errors.Is(err, ErrRootInstall) {
		t.Fatalf("Start error = %v, want ErrRootInstall", err)
	}
}

func TestSystemdUninstallKeepsUnitWhenDisableFails(t *testing.T) {
	unitPath := filepath.Join(t.TempDir(), "surge.service")
	if err := os.WriteFile(unitPath, []byte("unit"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runner := &fakeRunner{err: errors.New("systemctl failed")}
	m := &SystemdManager{unitPath: unitPath, runner: runner, uid: 1000}

	if err := m.Uninstall(context.Background()); err == nil {
		t.Fatal("expected disable failure")
	}
	if _, err := os.Stat(unitPath); err != nil {
		t.Fatalf("unit was removed after disable failure: %v", err)
	}
}

func TestSystemdUnitAbsentRecognizesExplicitManagerErrors(t *testing.T) {
	if !systemdUnitAbsent("Failed to disable unit: Unit surge.service does not exist") {
		t.Fatal("expected missing-unit error to be recognized")
	}
	if systemdUnitAbsent("Failed to connect to bus: permission denied") {
		t.Fatal("unexpected manager error treated as a missing unit")
	}
}

func TestSystemdUnitEscapesPercentPaths(t *testing.T) {
	analyzer, err := exec.LookPath("systemd-analyze")
	if err != nil {
		t.Skip("systemd-analyze is unavailable")
	}
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "surge%Z")
	if err := os.Symlink(executable, path); err != nil {
		t.Fatal(err)
	}
	m := &SystemdManager{executable: path}
	unitPath := filepath.Join(dir, "review.service")
	if err := os.WriteFile(unitPath, []byte(m.unitContents()), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(analyzer, "verify", unitPath).CombinedOutput(); err != nil {
		t.Fatalf("unit rejected: %v: %s", err, out)
	}
}

func TestSystemdUnitPreservesUserPaths(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config space%"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state space%"))
	manager, err := New("/opt/surge")
	if err != nil {
		t.Fatal(err)
	}
	unit := manager.(*SystemdManager).unitContents()
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME"} {
		want := "Environment=" + systemdQuote(key+"="+os.Getenv(key))
		if !strings.Contains(unit, want) {
			t.Fatalf("missing %s in unit:\n%s", key, unit)
		}
	}
}
