package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/userservice"
	"github.com/spf13/cobra"
)

var getUserServiceManager = func() (userservice.Manager, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve Surge executable: %w", err)
	}
	return userservice.New(executable)
}

func runUserServiceAction(action func(context.Context, userservice.Manager) error, message string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		manager, err := getUserServiceManager()
		if err != nil {
			return err
		}
		if err := action(cmd.Context(), manager); err != nil {
			return err
		}
		if message != "" {
			fmt.Fprintln(cmd.OutOrStdout(), message)
		}
		return nil
	}
}

var serviceCmd = &cobra.Command{Use: "service", Short: "Manage Surge as a user service"}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start the Surge user service",
	RunE: runUserServiceAction(func(ctx context.Context, manager userservice.Manager) error {
		return manager.Install(ctx)
	}, "User service installed and started successfully"),
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Stop and uninstall the Surge user service",
	RunE: runUserServiceAction(func(ctx context.Context, manager userservice.Manager) error {
		return manager.Uninstall(ctx)
	}, "User service uninstalled successfully"),
}

var serviceStartCmd = &cobra.Command{Use: "start", Short: "Start the Surge user service", RunE: runUserServiceAction(func(ctx context.Context, manager userservice.Manager) error { return manager.Start(ctx) }, "User service started successfully")}
var serviceStopCmd = &cobra.Command{Use: "stop", Short: "Stop the Surge user service", RunE: runUserServiceAction(func(ctx context.Context, manager userservice.Manager) error { return manager.Stop(ctx) }, "User service stopped successfully")}
var serviceRestartCmd = &cobra.Command{Use: "restart", Short: "Restart the Surge user service", RunE: runUserServiceAction(func(ctx context.Context, manager userservice.Manager) error { return manager.Restart(ctx) }, "User service restarted successfully")}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the Surge user service",
	RunE: runUserServiceAction(func(ctx context.Context, manager userservice.Manager) error {
		state, err := manager.Status(ctx)
		if err != nil {
			return err
		}
		switch state {
		case userservice.Running:
			pid := readPIDFile(config.GetRuntimeDir())
			port := readPortFile(config.GetRuntimeDir())
			fmt.Printf("User service is running (PID: %d, Port: %d)\n", pid, port)
		case userservice.Stopped:
			fmt.Println("User service is stopped")
		default:
			fmt.Println("User service is not installed")
		}
		return nil
	}, ""),
}

func isSystemServiceRunning() bool {
	manager, err := getUserServiceManager()
	if err != nil {
		return false
	}
	state, err := manager.Status(context.Background())
	return err == nil && state == userservice.Running
}

func init() {
	rootCmd.AddCommand(serviceCmd)
	serviceCmd.AddCommand(serviceInstallCmd, serviceUninstallCmd, serviceStartCmd, serviceStopCmd, serviceRestartCmd, serviceStatusCmd)
}
