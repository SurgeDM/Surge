//go:build linux && !android

package cmd

import (
	"context"
	"testing"

	"github.com/SurgeDM/Surge/internal/userservice"
)

type fakeUserServiceManager struct {
	state        userservice.State
	installCalls int
}

func (m *fakeUserServiceManager) Install(context.Context) error   { m.installCalls++; return nil }
func (m *fakeUserServiceManager) Uninstall(context.Context) error { return nil }
func (m *fakeUserServiceManager) Start(context.Context) error     { return nil }
func (m *fakeUserServiceManager) Stop(context.Context) error      { return nil }
func (m *fakeUserServiceManager) Restart(context.Context) error   { return nil }
func (m *fakeUserServiceManager) Status(context.Context) (userservice.State, error) {
	return m.state, nil
}

func TestLinuxServiceInstallUsesUserManager(t *testing.T) {
	original := getUserServiceManager
	manager := &fakeUserServiceManager{}
	getUserServiceManager = func() (userservice.Manager, error) { return manager, nil }
	t.Cleanup(func() { getUserServiceManager = original })

	if err := serviceInstallCmd.RunE(serviceInstallCmd, nil); err != nil {
		t.Fatalf("service install: %v", err)
	}
	if manager.installCalls != 1 {
		t.Fatalf("install calls = %d, want 1", manager.installCalls)
	}
}

func TestLinuxServiceRunningUsesUserManagerState(t *testing.T) {
	original := getUserServiceManager
	manager := &fakeUserServiceManager{state: userservice.Running}
	getUserServiceManager = func() (userservice.Manager, error) { return manager, nil }
	t.Cleanup(func() { getUserServiceManager = original })

	if !isSystemServiceRunning() {
		t.Fatal("running user service was not detected")
	}
}
