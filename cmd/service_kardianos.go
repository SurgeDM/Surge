//go:build !android

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

var serviceConfig = &service.Config{
	Name:        "surge",
	DisplayName: "Surge Download Manager",
	Description: "Blazing fast TUI download manager built in Go.",
	Arguments:   []string{"service", "__run"},
}

type program struct {
	exit   chan struct{}
	cancel context.CancelFunc
	errCh  chan error
}

func (p *program) Start(s service.Service) error {
	// We run rootCmd.Execute() directly in a goroutine rather than starting
	// a subprocess to ensure the service manager tracks the correct PID
	// and to allow for shared state/lifecycle management if needed.
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.exit = make(chan struct{})
	p.errCh = make(chan error, 1)

	go func() {
		defer close(p.exit)
		// Re-enter cobra as `server start` instead of replaying os.Args
		// (which would re-match __run and recurse into RunService). This
		// permanently overrides rootCmd's args for the rest of the process
		// lifetime, which is fine: the process is owned by the service
		// manager from here until shutdown.
		rootCmd.SetArgs([]string{"server", "start", "--is-system-service"})
		if err := rootCmd.ExecuteContext(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Service error: %v\n", err)
			p.errCh <- err
			// Notify the service manager that the service should stop.
			// Use a goroutine to avoid deadlock on Windows where s.Stop()
			// might wait for p.Stop() to return.
			go func() { _ = s.Stop() }()
		}
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	// Gracefully stop the service by canceling the context.
	if p.cancel != nil {
		p.cancel()
	}
	if p.exit != nil {
		<-p.exit
	}

	// Return the error that caused the stop if any, so the service manager logs it.
	select {
	case err := <-p.errCh:
		return err
	default:
		return nil
	}
}

// GetService is a var so tests can swap in a mock without touching the OS
// service manager.  Tests that reassign this must NOT use t.Parallel()
// because the variable is package-scoped.
var GetService = func() (service.Service, error) {
	prg := &program{}
	return service.New(prg, serviceConfig)
}

func runAction(action func(service.Service) error, successMsg string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		if err := action(s); err != nil {
			return err
		}
		if successMsg != "" {
			fmt.Println(successMsg)
		}
		return nil
	}
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Surge as a system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		if err := s.Install(); err != nil {
			return err
		}
		// Pre-generate the service token in the system state directory so
		// that `surge token` / `surge connect` can discover it without users
		// having to use --token manually.
		token, tokenErr := ensureSystemToken()
		fmt.Println("Service installed successfully")
		if tokenErr == nil {
			fmt.Printf("Service auth token: %s\n", token)
			fmt.Println("Use 'surge service token' to print this token again.")
		} else {
			fmt.Printf("Warning: could not persist service token: %v\n", tokenErr)
			fmt.Println("Run 'sudo surge service token' after starting the service to retrieve it.")
		}
		return nil
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the Surge system service",
	RunE: runAction(func(s service.Service) error {
		// Best effort stop before uninstall (Windows SCM rejects uninstall of running service)
		_ = s.Stop()
		return s.Uninstall()
	}, "Service uninstalled successfully"),
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Surge system service",
	RunE:  runAction(func(s service.Service) error { return s.Start() }, "Service started successfully"),
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Surge system service",
	RunE:  runAction(func(s service.Service) error { return s.Stop() }, "Service stopped successfully"),
}

// isSystemServiceRunning checks if the Kardianos service is currently running.
func isSystemServiceRunning() bool {
	s, err := GetService()
	if err != nil {
		return false
	}
	status, err := s.Status()
	return err == nil && status == service.StatusRunning
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the Surge system service",
	RunE: runAction(func(s service.Service) error {
		status, err := s.Status()
		if err != nil {
			return err
		}
		switch status {
		case service.StatusRunning:
			pid := readPIDFile(config.GetSystemRuntimeDir())
			port := readPortFile(config.GetSystemRuntimeDir())
			if pid > 0 && port > 0 {
				fmt.Printf("Service is running (PID: %d, Port: %d)\n", pid, port)
			} else if pid > 0 {
				fmt.Printf("Service is running (PID: %d)\n", pid)
			} else if port > 0 {
				fmt.Printf("Service is running (Port: %d)\n", port)
			} else {
				fmt.Println("Service is running")
			}
		case service.StatusStopped:
			fmt.Println("Service is stopped")
		default:
			fmt.Println("Service is not installed or status is unknown")
		}
		return nil
	}, ""),
}

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage Surge as a system service",
}

// serviceTokenCmd prints the auth token used by the system service daemon.
var serviceTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print the auth token used by the system service daemon",
	Long: `Print the auth token that the Surge system service uses.

The system service (installed with 'surge service install') stores its token
separately from the interactive-user token. Use this command to retrieve it
when connecting via 'surge connect' or setting up the browser extension.

Note: reading the system token file may require elevated privileges (sudo).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		token, err := readSystemServiceToken()
		if err != nil {
			var hint string
			if !isElevated() {
				hint = " (try running with sudo/administrator privileges)"
			}
			return fmt.Errorf("could not read system service token%s: %w", hint, err)
		}
		fmt.Println(token)
		return nil
	},
}

// __run is the entry point the installer writes into ExecStart
var serviceRunCmd = &cobra.Command{
	Use:    "__run",
	Hidden: true,
	RunE:   func(cmd *cobra.Command, args []string) error { return RunService() },
}

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
	serviceCmd.AddCommand(serviceTokenCmd)
	serviceCmd.AddCommand(serviceRunCmd)
}
