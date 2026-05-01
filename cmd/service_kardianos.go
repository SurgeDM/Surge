//go:build !android

package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

var serviceConfig = &service.Config{
	Name:        "surge",
	DisplayName: "Surge Download Manager",
	Description: "Blazing fast TUI download manager built in Go.",
	Arguments:   []string{"server", "start"},
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

func GetService() (service.Service, error) {
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
	RunE:  runAction(func(s service.Service) error { return s.Install() }, "Service installed successfully"),
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
			fmt.Println("Service is running")
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

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
}
