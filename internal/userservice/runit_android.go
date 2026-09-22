//go:build android

package userservice

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type runitRunner interface {
	Run(context.Context, string, []string, []string) ([]byte, error)
}
type runitExecRunner struct{}

func (runitExecRunner) Run(ctx context.Context, name string, args, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	return cmd.CombinedOutput()
}

type RunitManager struct {
	executable, prefix, baseDir, serviceDir string
	runner                                  runitRunner
}

func New(executable string) (Manager, error) {
	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		prefix = "/data/data/com.termux/files/usr"
	}
	base := os.Getenv("SURGE_SV_DIR")
	if base == "" {
		base = os.Getenv("SVDIR")
	}
	if base == "" {
		base = filepath.Join(prefix, "var", "service")
	}
	return &RunitManager{executable: executable, prefix: prefix, baseDir: base, serviceDir: filepath.Join(base, "surge"), runner: runitExecRunner{}}, nil
}

func (m *RunitManager) Install(ctx context.Context) error {
	if err := ensureRunitUser(); err != nil {
		return err
	}
	if _, err := exec.LookPath("sv"); err != nil {
		return fmt.Errorf("termux-services is unavailable; install it with 'pkg install termux-services'")
	}
	if err := os.MkdirAll(filepath.Join(m.serviceDir, "log"), 0o755); err != nil {
		return err
	}
	run := "#!" + filepath.Join(m.prefix, "bin", "sh") + "\nexec " + shellQuote(m.executable) + " server start --managed-service\n"
	if err := writeUserServiceFile(filepath.Join(m.serviceDir, "run"), []byte(run), 0o755); err != nil {
		return err
	}
	logRun := filepath.Join(m.serviceDir, "log", "run")
	_ = os.Remove(logRun)
	if err := os.Symlink(filepath.Join(m.prefix, "share", "termux-services", "svlogger"), logRun); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(m.serviceDir, "down"))
	return m.run(ctx, "up")
}
func (m *RunitManager) Uninstall(ctx context.Context) error {
	if err := ensureRunitUser(); err != nil {
		return err
	}
	_, _ = m.command(ctx, "down")
	return os.RemoveAll(m.serviceDir)
}
func (m *RunitManager) Start(ctx context.Context) error {
	if err := ensureRunitUser(); err != nil {
		return err
	}
	return m.run(ctx, "up")
}
func (m *RunitManager) Stop(ctx context.Context) error {
	if err := ensureRunitUser(); err != nil {
		return err
	}
	return m.run(ctx, "down")
}
func (m *RunitManager) Restart(ctx context.Context) error {
	if err := ensureRunitUser(); err != nil {
		return err
	}
	return m.run(ctx, "restart")
}
func (m *RunitManager) Status(ctx context.Context) (State, error) {
	if err := ensureRunitUser(); err != nil {
		return NotInstalled, err
	}
	if _, err := os.Stat(m.serviceDir); os.IsNotExist(err) {
		return NotInstalled, nil
	} else if err != nil {
		return NotInstalled, err
	}
	out, err := m.command(ctx, "status")
	if err != nil {
		return Stopped, nil
	}
	if strings.HasPrefix(strings.TrimSpace(string(out)), "run:") {
		return Running, nil
	}
	return Stopped, nil
}
func ensureRunitUser() error {
	if os.Geteuid() == 0 {
		return ErrRootInstall
	}
	return nil
}
func (m *RunitManager) command(ctx context.Context, action string) ([]byte, error) {
	env := os.Environ()
	if dir := os.Getenv("SURGE_SV_DIR"); dir != "" {
		env = append(env, "SVDIR="+dir)
	}
	return m.runner.Run(ctx, "sv", []string{action, "surge"}, env)
}
func (m *RunitManager) run(ctx context.Context, action string) error {
	out, err := m.command(ctx, action)
	if err != nil {
		return fmt.Errorf("sv %s surge failed: %s", action, strings.TrimSpace(string(out)))
	}
	return nil
}
func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
