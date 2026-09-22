//go:build darwin

package userservice

import (
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const launchdLabel = "com.surgedm.surge"

type launchdRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type launchdExecRunner struct{}

func (launchdExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

type LaunchdManager struct {
	executable string
	plistPath  string
	home       string
	uid        int
	runner     launchdRunner
}

func New(executable string) (Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}
	return &LaunchdManager{
		executable: executable,
		plistPath:  filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist"),
		home:       home,
		uid:        os.Geteuid(),
		runner:     launchdExecRunner{},
	}, nil
}

func (m *LaunchdManager) target() string { return "gui/" + strconv.Itoa(m.uid) + "/" + launchdLabel }
func (m *LaunchdManager) domain() string { return "gui/" + strconv.Itoa(m.uid) }

func (m *LaunchdManager) Install(ctx context.Context) error {
	if err := m.ensureUser(); err != nil {
		return err
	}
	for _, legacy := range []string{"/Library/LaunchDaemons/com.surgedm.surge.plist", "/Library/LaunchDaemons/surge.plist"} {
		if _, err := os.Stat(legacy); err == nil {
			return fmt.Errorf("%w at %s; unload and remove it before retrying without sudo", ErrLegacySystemService, legacy)
		}
	}
	if err := os.MkdirAll(filepath.Dir(m.plistPath), 0o755); err != nil {
		return err
	}
	if err := writeUserServiceFile(m.plistPath, []byte(m.plist()), 0o644); err != nil {
		return err
	}
	_, _ = m.runner.Run(ctx, "launchctl", "bootout", m.target())
	if err := m.run(ctx, "bootstrap", m.domain(), m.plistPath); err != nil {
		return err
	}
	return m.run(ctx, "enable", m.target())
}

func (m *LaunchdManager) Uninstall(ctx context.Context) error {
	if err := m.ensureUser(); err != nil {
		return err
	}
	_, _ = m.runner.Run(ctx, "launchctl", "bootout", m.target())
	if err := os.Remove(m.plistPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func (m *LaunchdManager) Start(ctx context.Context) error {
	if err := m.ensureUser(); err != nil {
		return err
	}
	return m.run(ctx, "kickstart", "-k", m.target())
}
func (m *LaunchdManager) Stop(ctx context.Context) error {
	if err := m.ensureUser(); err != nil {
		return err
	}
	return m.run(ctx, "kill", "SIGTERM", m.target())
}
func (m *LaunchdManager) Restart(ctx context.Context) error { return m.Start(ctx) }
func (m *LaunchdManager) Status(ctx context.Context) (State, error) {
	if err := m.ensureUser(); err != nil {
		return NotInstalled, err
	}
	if _, err := os.Stat(m.plistPath); os.IsNotExist(err) {
		return NotInstalled, nil
	} else if err != nil {
		return NotInstalled, err
	}
	if _, err := m.runner.Run(ctx, "launchctl", "print", m.target()); err != nil {
		return Stopped, nil
	}
	return Running, nil
}
func (m *LaunchdManager) ensureUser() error {
	if m.uid == 0 {
		return ErrRootInstall
	}
	return nil
}
func (m *LaunchdManager) run(ctx context.Context, args ...string) error {
	out, err := m.runner.Run(ctx, "launchctl", args...)
	if err != nil {
		return fmt.Errorf("launchctl %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}
func (m *LaunchdManager) plist() string {
	e := html.EscapeString(m.executable)
	h := html.EscapeString(m.home)
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>` + launchdLabel + `</string>
<key>ProgramArguments</key><array><string>` + e + `</string><string>server</string><string>start</string><string>--managed-service</string></array>
<key>WorkingDirectory</key><string>` + h + `</string>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
</dict></plist>
`
}
