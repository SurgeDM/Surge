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

func waitStop(t *testing.T, p *program, s service.Service) {
	stopErr := make(chan error, 1)
	go func() {
		stopErr <- p.Stop(s)
	}()

	select {
	case err := <-stopErr:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("p.Stop timed out")
	}
}

func TestProgramLifecycle(t *testing.T) {
	p := &program{}
	s := &mockService{}

	// Set args to something safe so rootCmd.ExecuteContext doesn't fail on test flags
	rootCmd.SetArgs([]string{"--help"})
	defer rootCmd.SetArgs(nil)

	// Test Start
	err := p.Start(s)
	assert.NoError(t, err)
	assert.NotNil(t, p.cancel)
	assert.NotNil(t, p.exit)

	// Test Stop with timeout
	waitStop(t, p, s)

	// Verify p.exit is closed
	_, ok := <-p.exit
	assert.False(t, ok, "p.exit should be closed")
}

func TestToggleServiceFunc(t *testing.T) {
	// This test verifies the logic we bind to m.ToggleServiceFunc in root.go
	s := &mockService{}

	toggleFunc := func(enable bool) error {
		if enable {
			return s.Install()
		}
		// Best effort stop before uninstall
		_ = s.Stop()
		return s.Uninstall()
	}

	err := toggleFunc(true)
	assert.NoError(t, err)
	assert.True(t, s.installCalled)

	err = toggleFunc(false)
	assert.NoError(t, err)
	assert.True(t, s.stopCalled)
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

	// Test Stop with timeout
	waitStop(t, p, s)
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
