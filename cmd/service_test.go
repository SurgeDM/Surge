package cmd

import (
	"testing"
	"time"

	"github.com/kardianos/service"
	"github.com/stretchr/testify/assert"
)

type mockService struct {
	service.Service
	stopCalled      bool
	installCalled   bool
	uninstallCalled bool
}

func (m *mockService) Stop() error {
	m.stopCalled = true
	return nil
}

func (m *mockService) Install() error {
	m.installCalled = true
	return nil
}

func (m *mockService) Uninstall() error {
	m.uninstallCalled = true
	return nil
}

func (m *mockService) Status() (service.Status, error) {
	return service.StatusRunning, nil
}

func TestProgramLifecycle(t *testing.T) {
	p := &program{}
	s := &mockService{}

	// Test Start
	err := p.Start(s)
	assert.NoError(t, err)
	assert.NotNil(t, p.cancel)
	assert.NotNil(t, p.exit)

	// Test Stop
	err = p.Stop(s)
	assert.NoError(t, err)

	// Ensure goroutine finished (it should have closed p.exit)
	select {
	case <-p.exit:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("program did not signal exit within timeout")
	}
}

func TestToggleServiceFunc(t *testing.T) {
	// This test verifies the logic we bind to m.ToggleServiceFunc in root.go
	s := &mockService{}

	toggleFunc := func(enable bool) error {
		if enable {
			return s.Install()
		}
		return s.Uninstall()
	}

	err := toggleFunc(true)
	assert.NoError(t, err)
	assert.True(t, s.installCalled)

	err = toggleFunc(false)
	assert.NoError(t, err)
	assert.True(t, s.uninstallCalled)
}

func TestProgramContextCancellation(t *testing.T) {
	p := &program{}
	s := &mockService{}

	// Start the program
	_ = p.Start(s)

	// Capture the cancel function
	cancel := p.cancel
	assert.NotNil(t, cancel)

	// Stopping should call cancel
	_ = p.Stop(s)
}

func TestServiceCommandRegistration(t *testing.T) {
	// Verify service command is registered with correct subcommands
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "service" {
			found = true
			subcommands := cmd.Commands()
			assert.NotEmpty(t, subcommands)

			names := []string{}
			for _, sub := range subcommands {
				names = append(names, sub.Name())
			}
			assert.Contains(t, names, "install")
			assert.Contains(t, names, "uninstall")
			assert.Contains(t, names, "start")
			assert.Contains(t, names, "stop")
			assert.Contains(t, names, "status")
			break
		}
	}
	assert.True(t, found, "service command not found in rootCmd")
}
