//go:build linux && !android

package userservice

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SurgeDM/Surge/internal/config"
)

const systemdUnitName = "surge.service"

var ErrRootInstall = errors.New("Surge user services cannot be managed as root; run this command again without sudo")
var ErrLegacySystemService = errors.New("a legacy system-wide Surge service is installed")

type commandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type SystemdManager struct {
	executable  string
	unitPath    string
	runner      commandRunner
	uid         int
	legacyCheck func() bool
}

func New(executable string) (Manager, error) {
	if strings.TrimSpace(executable) == "" {
		return nil, errors.New("service executable path is empty")
	}
	return &SystemdManager{
		executable:  executable,
		unitPath:    filepath.Join(filepath.Dir(config.GetSurgeDir()), "systemd", "user", systemdUnitName),
		runner:      execRunner{},
		uid:         os.Geteuid(),
		legacyCheck: legacySystemdUnitExists,
	}, nil
}

func (m *SystemdManager) Install(ctx context.Context) error {
	if m.uid == 0 {
		return ErrRootInstall
	}
	legacyCheck := m.legacyCheck
	if legacyCheck == nil {
		legacyCheck = legacySystemdUnitExists
	}
	if legacyCheck() {
		return fmt.Errorf("%w; stop and remove it before installing the user service", ErrLegacySystemService)
	}
	if err := os.MkdirAll(filepath.Dir(m.unitPath), 0o755); err != nil {
		return fmt.Errorf("create systemd user directory: %w", err)
	}
	if err := writeAtomic(m.unitPath, []byte(m.unitContents()), 0o644); err != nil {
		return fmt.Errorf("write systemd user unit: %w", err)
	}
	if err := m.run(ctx, "daemon-reload"); err != nil {
		return err
	}
	return m.run(ctx, "enable", "--now", systemdUnitName)
}

func (m *SystemdManager) Uninstall(ctx context.Context) error {
	if m.uid == 0 {
		return ErrRootInstall
	}
	_, _ = m.runner.Run(ctx, "systemctl", "--user", "disable", "--now", systemdUnitName)
	if err := os.Remove(m.unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove systemd user unit: %w", err)
	}
	return m.run(ctx, "daemon-reload")
}

func (m *SystemdManager) Start(ctx context.Context) error {
	return m.run(ctx, "start", systemdUnitName)
}

func (m *SystemdManager) Stop(ctx context.Context) error {
	return m.run(ctx, "stop", systemdUnitName)
}

func (m *SystemdManager) Restart(ctx context.Context) error {
	return m.run(ctx, "restart", systemdUnitName)
}

func (m *SystemdManager) Status(ctx context.Context) (State, error) {
	if _, err := os.Stat(m.unitPath); os.IsNotExist(err) {
		return NotInstalled, nil
	} else if err != nil {
		return NotInstalled, err
	}
	out, err := m.runner.Run(ctx, "systemctl", "--user", "is-active", systemdUnitName)
	if strings.TrimSpace(string(out)) == "active" {
		return Running, nil
	}
	if err != nil {
		return Stopped, nil
	}
	return Stopped, nil
}

func (m *SystemdManager) run(ctx context.Context, args ...string) error {
	out, err := m.runner.Run(ctx, "systemctl", append([]string{"--user"}, args...)...)
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("systemctl --user %s failed: %s", strings.Join(args, " "), message)
	}
	return nil
}

func (m *SystemdManager) unitContents() string {
	home, _ := os.UserHomeDir()
	return "[Unit]\n" +
		"Description=Surge Download Manager\n" +
		"Wants=network-online.target\n" +
		"After=network-online.target\n\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"ExecStart=" + systemdQuote(m.executable) + " server start --managed-service\n" +
		"WorkingDirectory=" + systemdQuote(home) + "\n" +
		"Restart=on-failure\n" +
		"RestartSec=5\n\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}

func systemdQuote(value string) string {
	return strconv.Quote(value)
}

func legacySystemdUnitExists() bool {
	for _, path := range []string{
		"/etc/systemd/system/surge.service",
		"/usr/lib/systemd/system/surge.service",
		"/lib/systemd/system/surge.service",
	} {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".surge-service-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
