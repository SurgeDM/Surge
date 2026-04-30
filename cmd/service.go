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
}

func (p *program) Start(s service.Service) error {
	// We run rootCmd.Execute() directly in a goroutine rather than starting
	// a subprocess to ensure the service manager tracks the correct PID
	// and to allow for shared state/lifecycle management if needed.
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.exit = make(chan struct{})

	go func() {
		defer close(p.exit)
		if err := rootCmd.ExecuteContext(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Service error: %v\n", err)
			// Notify the service manager that the service has stopped due to error.
			_ = s.Stop()
		}
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	// Gracefully stop the service by canceling the context.
	// This works cross-platform (including Windows) where os.Interrupt signal may not be supported.
	if p.cancel != nil {
		p.cancel()
	}
	if p.exit != nil {
		<-p.exit
	}
	return nil
}

func GetService() (service.Service, error) {
	prg := &program{}
	return service.New(prg, serviceConfig)
}

// RunService handles the application execution, checking if it should run as a service.
func RunService() error {
	s, err := GetService()
	if err != nil {
		return rootCmd.Execute()
	}

	if service.Interactive() {
		return rootCmd.Execute()
	}

	return s.Run()
}

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage Surge as a system service",
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Surge as a system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		err = s.Install()
		if err == nil {
			fmt.Println("Service installed successfully")
		}
		return err
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the Surge system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		err = s.Uninstall()
		if err == nil {
			fmt.Println("Service uninstalled successfully")
		}
		return err
	},
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Surge system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		err = s.Start()
		if err == nil {
			fmt.Println("Service started successfully")
		}
		return err
	},
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Surge system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		err = s.Stop()
		if err == nil {
			fmt.Println("Service stopped successfully")
		}
		return err
	},
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the Surge system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := GetService()
		if err != nil {
			return err
		}
		status, err := s.Status()
		if err != nil {
			return err
		}
		switch status {
		case service.StatusRunning:
			fmt.Println("Service is running")
		case service.StatusStopped:
			fmt.Println("Service is stopped")
		case service.StatusUnknown:
			fmt.Println("Service status: unknown")
		default:
			fmt.Println("Service is not installed")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceInstallCmd)
	serviceCmd.AddCommand(serviceUninstallCmd)
	serviceCmd.AddCommand(serviceStartCmd)
	serviceCmd.AddCommand(serviceStopCmd)
	serviceCmd.AddCommand(serviceStatusCmd)
}
