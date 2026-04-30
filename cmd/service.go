package cmd

import (
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

type program struct{}

func (p *program) Start(s service.Service) error {
	// The service library handles the running of the executable with the specified arguments.
	// When running as a service, Surge is invoked with "server start".
	go func() {
		if err := rootCmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Service error: %v\n", err)
			os.Exit(1)
		}
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	// Stop should be graceful. Surge handles SIGTERM/SIGINT.
	proc, err := os.FindProcess(os.Getpid())
	if err == nil {
		_ = proc.Signal(os.Interrupt)
	}
	return nil
}

func getService() (service.Service, error) {
	prg := &program{}
	return service.New(prg, serviceConfig)
}

// RunService handles the application execution, checking if it should run as a service.
func RunService() error {
	s, err := getService()
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
		s, err := getService()
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
		s, err := getService()
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
		s, err := getService()
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
		s, err := getService()
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
		s, err := getService()
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
		default:
			fmt.Println("Service status: unknown")
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
