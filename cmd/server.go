package cmd

import (
	"fmt"

	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/utils"
)

var serverCmd = &cobra.Command{
	Use:   "server [url]...",
	Short: "Manage the Surge background server (daemon)",
	Long:  `Run the Surge background server in headless mode.`,
	Run: func(cmd *cobra.Command, args []string) {
		serverStartCmd.Run(cmd, args)
	},
}

var serverStartCmd = &cobra.Command{
	Use:   "start [url]...",
	Short: "Start the Surge server in headless mode",
	Run: func(cmd *cobra.Command, args []string) {
		// Attempt to acquire lock before any global state initialization
		isMaster, err := AcquireLock()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error acquiring lock: %v\n", err)
			os.Exit(1)
		}

		if !isMaster {
			fmt.Fprintln(os.Stderr, "Error: Surge server is already running.")
			os.Exit(1)
		}
		defer func() {
			if err := ReleaseLock(); err != nil {
				utils.Debug("Error releasing lock: %v", err)
			}
		}()

		mustInitializeGlobalState()

		msg := runStartupIntegrityCheck()
		if msg != "" {
			utils.Debug("%s", msg)
			fmt.Println(msg)
		}

		portFlag, _ := cmd.Flags().GetInt("port")
		batchFile, _ := cmd.Flags().GetString("batch")
		outputDir, _ := cmd.Flags().GetString("output")
		exitWhenDone, _ := cmd.Flags().GetBool("exit-when-done")
		noResume, _ := cmd.Flags().GetBool("no-resume")

		// Save current PID to file
		savePID()
		defer removePID()

		// Get token flag
		tokenFlag := resolveServerToken(cmd)
		startServerLogic(cmd, args, portFlag, batchFile, outputDir, exitWhenDone, noResume, tokenFlag)
	},
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running Surge server",
	Run: func(cmd *cobra.Command, args []string) {
		pid := readPID()
		if pid == 0 {
			fmt.Println("No running Surge server found (PID file missing).")
			return
		}

		process, err := os.FindProcess(pid)
		if err != nil {
			fmt.Printf("Error finding process: %v\n", err)
			return
		}

		// Try to send SIGTERM
		err = process.Signal(syscall.SIGTERM)
		if err != nil {
			fmt.Printf("Error stopping server: %v\n", err)
			return
		}

		fmt.Printf("Sent stop signal to process %d\n", pid)
	},
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of the Surge server",
	Run: func(cmd *cobra.Command, args []string) {
		pid := readPID()
		if pid == 0 {
			fmt.Println("Surge server is NOT running.")
			return
		}

		// Check if process exists
		process, err := os.FindProcess(pid)
		if err != nil {
			fmt.Printf("Surge server is NOT running (Process %d not found).\n", pid)
			// Cleanup stale pid file?
			return
		}

		// Sending signal 0 to check existence
		err = process.Signal(syscall.Signal(0))
		if err != nil {
			fmt.Printf("Surge server is NOT running (Process %d dead).\n", pid)
			return
		}

		port := readActivePort()
		fmt.Printf("Surge server is running (PID: %d, Port: %d).\n", pid, port)
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.AddCommand(serverStartCmd)
	serverCmd.AddCommand(serverStopCmd)
	serverCmd.AddCommand(serverStatusCmd)

	serverCmd.PersistentFlags().StringP("batch", "b", "", "File containing URLs to download")
	serverCmd.PersistentFlags().IntP("port", "p", 0, "Port to listen on")
	serverCmd.PersistentFlags().StringP("output", "o", "", "Output directory (defaults to current working directory)")
	serverCmd.PersistentFlags().Bool("exit-when-done", false, "Exit when all downloads complete")
	serverCmd.PersistentFlags().Bool("no-resume", false, "Do not auto-resume paused downloads on startup")
	serverCmd.PersistentFlags().String("token", "", "Auth token for API clients (or set SURGE_TOKEN)")
}

func savePID() {
	pid := os.Getpid()
	if err := os.MkdirAll(config.GetRuntimeDir(), 0o755); err != nil {
		utils.Debug("Error creating runtime directory for PID file: %v", err)
		return
	}
	pidFile := filepath.Join(config.GetRuntimeDir(), "pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0o644); err != nil {
		utils.Debug("Error writing PID file: %v", err)
	}
}

func removePID() {
	pidFile := filepath.Join(config.GetRuntimeDir(), "pid")
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		utils.Debug("Error removing PID file: %v", err)
	}
}

func readPID() int {
	pidFile := filepath.Join(config.GetRuntimeDir(), "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(string(data))
	return pid
}

func startServerLogic(cmd *cobra.Command, args []string, portFlag int, batchFile string, outputDir string, exitWhenDone bool, noResume bool, tokenOverride string) {
	port, listener, err := bindServerListener(portFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	resetGlobalEnqueueContext()

	if err := ensureGlobalLocalServiceAndLifecycle(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating lifecycle event stream: %v\n", err)
		os.Exit(1)
	}

	saveActivePort(port)
	defer removeActivePort()

	go startHTTPServer(listener, port, outputDir, GlobalService, strings.TrimSpace(tokenOverride))

	queueInitialRootDownloads(args, rootRunOptions{
		batchFile: batchFile,
		outputDir: outputDir,
	})

	fmt.Printf("Surge %s running in server mode.\n", Version)
	host := serverBindHost
	fmt.Printf("Serving on %s:%d\n", host, port)
	fmt.Println("Press Ctrl+C to exit.")

	StartHeadlessConsumer()

	// Auto-resume paused downloads (unless --no-resume)
	if !noResume {
		resumePausedDownloads()
	}

	if exitWhenDone {
		exitWhenDoneCh := make(chan struct{}, 1)
		go func() {
			time.Sleep(2 * time.Second)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if atomic.LoadInt32(&pendingEnqueue) == 0 && atomic.LoadInt32(&activeDownloads) == 0 {
					if GlobalPool != nil && GlobalPool.ActiveCount() == 0 {
						select {
						case exitWhenDoneCh <- struct{}{}:
						default:
						}
						return
					}
				}
			}
		}()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		defer signal.Stop(sigChan)

		select {
		case sig := <-sigChan:
			fmt.Printf("\nReceived %s. Shutting down...\n", sig)
			_ = executeGlobalShutdown(fmt.Sprintf("server signal: %s", sig))
		case <-exitWhenDoneCh:
			fmt.Println("All downloads finished. Exiting...")
			_ = executeGlobalShutdown("server: exit when done")
		}
		return
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigChan)
	sig := <-sigChan

	fmt.Printf("\nReceived %s. Shutting down...\n", sig)
	_ = executeGlobalShutdown(fmt.Sprintf("server signal: %s", sig))
}

func resolveServerToken(cmd *cobra.Command) string {
	var tokenFlag string
	if f := cmd.Flag("token"); f != nil {
		tokenFlag = f.Value.String()
	}
	if token := strings.TrimSpace(tokenFlag); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("SURGE_TOKEN"))
}
