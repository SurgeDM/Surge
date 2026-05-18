//go:build android

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// termuxServiceDir returns the runit service directory for surge.
// SURGE_SV_DIR overrides the base SVDIR (like SVDIR), not the full path,
// so that sv commands can resolve the service by name.
func termuxServiceDir() string {
	return filepath.Join(svBaseDir(), "surge")
}

// svServiceName returns the short service name that sv resolves via $SVDIR.
func svServiceName() string {
	return "surge"
}

// defaultPrefix returns the Termux prefix path.
func defaultPrefix() string {
	if p := os.Getenv("PREFIX"); p != "" {
		return p
	}
	return "/data/data/com.termux/files/usr"
}

// svBaseDir returns the base service directory.
// SURGE_SV_DIR takes precedence over SVDIR for Surge's service management.
func svBaseDir() string {
	if dir := os.Getenv("SURGE_SV_DIR"); dir != "" {
		return dir
	}
	if svDir := os.Getenv("SVDIR"); svDir != "" {
		return svDir
	}
	return filepath.Join(defaultPrefix(), "var", "service")
}

// termuxServiceRunScript returns the content of the run script for the surge service.
func termuxServiceRunScript() string {
	exe, _ := os.Executable()
	if exe == "" {
		exe = "surge"
	}
	return "#!/" + defaultPrefix() + "/bin/sh\nexec " + exe + " server start\n"
}

// sv runs an sv command and returns its output.
// If SURGE_SV_DIR is set, it is passed as SVDIR to the sv command
// so that the service name resolves correctly.
func sv(args ...string) (string, error) {
	svcmd := exec.Command("sv", args...)
	env := os.Environ()
	if dir := os.Getenv("SURGE_SV_DIR"); dir != "" {
		filtered := env[:0:len(env)]
		for _, e := range env {
			if !strings.HasPrefix(e, "SVDIR=") {
				filtered = append(filtered, e)
			}
		}
		env = append(filtered, "SVDIR="+dir)
	}
	svcmd.Env = env
	out, err := svcmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// isTermuxServicesAvailable checks if runit/termux-services is set up.
func isTermuxServicesAvailable() bool {
	if _, err := exec.LookPath("sv"); err != nil {
		return false
	}
	info, err := os.Stat(svBaseDir())
	if err != nil || !info.IsDir() {
		return false
	}
	return true
}

// writeRunScript writes a service run script with executable permissions.
func writeRunScript(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o755)
}

var serviceInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Surge as a system service (Termux/runit)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isTermuxServicesAvailable() {
			return fmt.Errorf("termux-services is not available. Install it with: pkg install termux-services")
		}

		svcDir := termuxServiceDir()
		if info, err := os.Stat(svcDir); err == nil && info.IsDir() {
			return fmt.Errorf("service already installed at %s. Run 'surge service uninstall' first", svcDir)
		}

		if err := installTermuxService(); err != nil {
			return err
		}

		fmt.Println("Service installed successfully")
		fmt.Printf("Service directory: %s\n", svcDir)
		fmt.Println("Use 'surge service start' to start the service")
		return nil
	},
}

var serviceUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall the Surge system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcDir := termuxServiceDir()
		if _, err := os.Stat(svcDir); os.IsNotExist(err) {
			return fmt.Errorf("service is not installed")
		}

		if err := uninstallTermuxService(); err != nil {
			return err
		}

		fmt.Println("Service uninstalled successfully")
		return nil
	},
}

var serviceStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Surge system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isTermuxServicesAvailable() {
			return fmt.Errorf("termux-services is not available. Install with: pkg install termux-services")
		}

		svcDir := termuxServiceDir()
		if _, err := os.Stat(svcDir); os.IsNotExist(err) {
			return fmt.Errorf("service is not installed. Run 'surge service install' first")
		}

		out, err := sv("up", svServiceName())
		if err != nil {
			return fmt.Errorf("failed to start service: %s (%w)", out, err)
		}

		fmt.Println("Service started successfully")
		return nil
	},
}

var serviceStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Surge system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		svcDir := termuxServiceDir()
		if _, err := os.Stat(svcDir); os.IsNotExist(err) {
			return fmt.Errorf("service is not installed")
		}

		out, err := sv("down", svServiceName())
		if err != nil {
			return fmt.Errorf("failed to stop service: %s (%w)", out, err)
		}

		fmt.Println("Service stopped successfully")
		return nil
	},
}

var serviceStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the Surge system service",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isTermuxServicesAvailable() {
			fmt.Println("termux-services is not available. Install with: pkg install termux-services")
			return nil
		}

		svcDir := termuxServiceDir()
		if _, err := os.Stat(svcDir); os.IsNotExist(err) {
			fmt.Println("Service is not installed")
			return nil
		}

		out, _ := sv("status", svServiceName())
		fmt.Println(out)
		return nil
	},
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
