//go:build !android

package cmd

import (
	"fmt"
	"net"
	"runtime"
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
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		fmt.Printf("GOROUTINE DUMP:\n%s\n", buf[:n])
		t.Fatal("p.Stop timed out")
	}
}

func TestProgramLifecycle(t *testing.T) {
	setupIsolatedCmdState(t)

	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("Skipping test because tcp listener cannot bind (e.g. Windows dual-stack provider issue): %v", err)
	}
	_ = ln.Close()

	p := &program{}
	s := &mockService{}

	// Set args to something safe so rootCmd.ExecuteContext doesn't fail on test flags
	rootCmd.SetArgs([]string{"--help"})
	resetGlobalShutdownCoordinatorForTest(nil)
	defer func() {
		rootCmd.SetArgs(nil)
		resetGlobalShutdownCoordinatorForTest(nil)
	}()

	// Test Start
	err = p.Start(s)
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
	s := &mockService{}

	toggleFunc := func(enable bool) error {
		if enable {
			return s.Install()
		}
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
	setupIsolatedCmdState(t)

	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Skipf("Skipping test because tcp listener cannot bind (e.g. Windows dual-stack provider issue): %v", err)
	}
	_ = ln.Close()

	p := &program{}
	s := &mockService{}

	rootCmd.SetArgs([]string{"--help"})
	resetGlobalShutdownCoordinatorForTest(nil)
	defer func() {
		rootCmd.SetArgs(nil)
		resetGlobalShutdownCoordinatorForTest(nil)
	}()

	_ = p.Start(s)

	cancel := p.cancel
	assert.NotNil(t, cancel)

	waitStop(t, p, s)
}
