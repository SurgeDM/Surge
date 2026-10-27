package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server [url]...",
	Short: "Manage the Surge background server (daemon)",
	Long:  `Run the Surge background server in headless mode.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return serverStartCmd.RunE(cmd, args)
	},
}

var managedServiceFlag bool

var serverStartCmd = &cobra.Command{
	Use:   "start [url]...",
	Short: "Start the Surge server in headless mode",
	RunE: func(cmd *cobra.Command, args []string) error {
		if managedServiceFlag {
			activeServerMode = "service"
		}
		if checkUserServiceRunning() && !managedServiceFlag {
			return fmt.Errorf("the Surge user service is already running; use 'surge connect' or stop the service first")
		}

		// Attempt to acquire lock before any global state initialization
		lock, isMaster, err := TryAcquireInstanceLock(resolveRuntimeDir())
		if err != nil {
			return fmt.Errorf("error acquiring lock: %w", err)
		}

		if !isMaster {
			return fmt.Errorf("surge server is already running")
		}
		defer func() {
			if err := lock.Release(); err != nil {
				utils.Debug("Error releasing lock: %v", err)
			}
		}()

		if err := initializeLocalEngine(); err != nil {
			return err
		}

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
		return startServerLogic(cmd, args, portFlag, batchFile, outputDir, exitWhenDone, noResume, tokenFlag)
	},
}

var serverStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the running Surge server",
	RunE: func(cmd *cobra.Command, args []string) error {
		baseURL, token, err := resolveAPIConnection(true)
		if err != nil {
			return err
		}
		client, err := newRemoteAPIHTTPClient()
		if err != nil {
			return err
		}
		return requestServerShutdown(cmd, client, baseURL, token)
	},
}

func requestServerShutdown(cmd *cobra.Command, client *http.Client, baseURL, token string) error {
	if err := verifySurgeServer(client, baseURL); err != nil {
		return err
	}
	resp, err := doAPIRequestWithClient(client, http.MethodPost, baseURL, token, "/shutdown", nil)
	if err != nil {
		return fmt.Errorf("request server shutdown: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server rejected shutdown: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "Server shutdown requested.")
	return err
}

func verifySurgeServer(client *http.Client, baseURL string) error {
	resp, err := doAPIRequestWithClient(client, http.MethodGet, baseURL, "", "/health", nil)
	if err != nil {
		return fmt.Errorf("verify Surge server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("verify Surge server: health endpoint returned %s", resp.Status)
	}
	var health struct {
		Status string `json:"status"`
		Mode   string `json:"mode"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024)).Decode(&health); err != nil {
		return fmt.Errorf("verify Surge server: decode health response: %w", err)
	}
	if health.Status != "ok" || (health.Mode != "local" && health.Mode != "service") {
		return fmt.Errorf("verify Surge server: unexpected health response")
	}
	return nil
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

	serverStartCmd.Flags().BoolVar(&managedServiceFlag, "managed-service", false, "Internal flag for a user-managed service")
	_ = serverStartCmd.Flags().MarkHidden("managed-service")
}

func savePID() {
	pid := os.Getpid()
	if err := os.MkdirAll(resolveRuntimeDir(), 0o755); err != nil {
		utils.Debug("Error creating runtime directory for PID file: %v", err)
		return
	}
	pidFile := filepath.Join(resolveRuntimeDir(), "pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", pid)), 0o644); err != nil {
		utils.Debug("Error writing PID file: %v", err)
	}
}

func removePID() {
	pidFile := filepath.Join(resolveRuntimeDir(), "pid")
	if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
		utils.Debug("Error removing PID file: %v", err)
	}
}

func readPID() int {
	pidFile := filepath.Join(resolveRuntimeDir(), "pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(string(data))
	return pid
}

func startServerLogic(cmd *cobra.Command, args []string, portFlag int, batchFile string, outputDir string, exitWhenDone bool, noResume bool, tokenOverride string) error {
	port, listener, err := bindServerListener(portFlag)
	if err != nil {
		return err
	}
	resetGlobalEnqueueContext()

	if err := ensureGlobalLocalServiceAndLifecycle(); err != nil {
		return fmt.Errorf("error creating lifecycle event stream: %w", err)
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

	StartHeadlessConsumer(cmd.Root().Context(), GlobalService)

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
				if atomic.LoadInt32(&pendingEnqueue) == 0 {
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
		case <-cmd.Root().Context().Done():
			fmt.Printf("\nService stop requested. Shutting down...\n")
			_ = executeGlobalShutdown("server: service context cancelled")
		case reason := <-shutDownRequested:
			fmt.Printf("\n%s Shutting down...\n", reason)
			_ = executeGlobalShutdown(reason)
		case <-exitWhenDoneCh:
			fmt.Println("All downloads finished. Exiting...")
			_ = executeGlobalShutdown("server: exit when done")
		}
		return nil
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	select {
	case sig := <-sigChan:
		fmt.Printf("\nReceived %s. Shutting down...\n", sig)
		_ = executeGlobalShutdown(fmt.Sprintf("server signal: %s", sig))
	case <-cmd.Root().Context().Done():
		fmt.Printf("\nService stop requested. Shutting down...\n")
		_ = executeGlobalShutdown("server: service context cancelled")
	case reason := <-shutDownRequested:
		fmt.Printf("\nShutdown requested. Shutting down...\n")
		_ = executeGlobalShutdown(reason)
	}
	return nil
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
