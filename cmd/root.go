package cmd

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/download"
	"github.com/surge-downloader/surge/internal/engine/events"
	"github.com/surge-downloader/surge/internal/engine/state"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/processing"
	"github.com/surge-downloader/surge/internal/tui"
	"github.com/surge-downloader/surge/internal/utils"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// Version information - set via ldflags during build
var (
	Version   = "dev"
	BuildTime = "unknown"
)

func init() {
	// Override with build info if ldflags didn't inject a version
	if Version == "dev" || Version == "" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = strings.TrimPrefix(info.Main.Version, "v")
		}
	}
}

// activeDownloads tracks the number of currently running downloads in headless mode
var activeDownloads int32

// pendingEnqueue tracks the number of pending batch enqueues to avoid premature exit
var pendingEnqueue int32

// Command line flags
var (
	verbose     bool
	globalHost  string
	globalToken string
)

// Globals for Unified Backend
var (
	GlobalPool              *download.WorkerPool
	GlobalProgressCh        chan any
	GlobalService           core.DownloadService
	GlobalLifecycleCleanup  func()
	serverProgram           *tea.Program
	startupIntegrityMessage string
	globalSettings          *config.Settings
	GlobalLifecycle         *processing.LifecycleManager
	globalLifecycleMu       sync.Mutex
	globalEnqueueCtx        context.Context
	globalEnqueueCancel     context.CancelFunc
	globalEnqueueMu         sync.Mutex
)

func buildPoolIsNameActive(getAll func() []types.DownloadConfig) processing.IsNameActiveFunc {
	if getAll == nil {
		return nil
	}

	return func(dir, name string) bool {
		dir = utils.EnsureAbsPath(strings.TrimSpace(dir))
		name = strings.TrimSpace(name)
		if dir == "" || name == "" {
			return false
		}

		for _, cfg := range getAll() {
			existingName := strings.TrimSpace(cfg.Filename)
			existingDir := strings.TrimSpace(cfg.OutputPath)
			if cfg.DestPath != "" {
				existingDir = filepath.Dir(cfg.DestPath)
				if existingName == "" {
					existingName = filepath.Base(cfg.DestPath)
				}
			}
			if cfg.State != nil {
				if stateName := strings.TrimSpace(cfg.State.GetFilename()); stateName != "" {
					existingName = stateName
				}
				if stateDestPath := strings.TrimSpace(cfg.State.GetDestPath()); stateDestPath != "" {
					existingDir = filepath.Dir(stateDestPath)
					if existingName == "" {
						existingName = filepath.Base(stateDestPath)
					}
				}
			}
			if existingDir == "" || existingName == "" {
				continue
			}
			if utils.EnsureAbsPath(existingDir) == dir && existingName == name {
				return true
			}
		}
		return false
	}
}

func newLocalLifecycleManager(service core.DownloadService, getAll func() []types.DownloadConfig) *processing.LifecycleManager {
	var addFunc processing.AddDownloadFunc
	var addWithIDFunc processing.AddDownloadWithIDFunc
	if service != nil {
		addFunc = service.Add
		addWithIDFunc = service.AddWithID
	}

	return processing.NewLifecycleManager(addFunc, addWithIDFunc, buildPoolIsNameActive(getAll))
}

func startLifecycleEventWorker(service core.DownloadService, mgr *processing.LifecycleManager) (func(), error) {
	if service == nil || mgr == nil {
		return nil, nil
	}

	managerStream, managerCleanup, err := service.StreamEvents(context.Background())
	if err != nil {
		return nil, err
	}
	go mgr.StartEventWorker(managerStream)
	return managerCleanup, nil
}

func currentLifecycle() *processing.LifecycleManager {
	globalLifecycleMu.Lock()
	defer globalLifecycleMu.Unlock()
	return GlobalLifecycle
}

func resetGlobalEnqueueContext() {
	globalEnqueueMu.Lock()
	defer globalEnqueueMu.Unlock()
	if globalEnqueueCancel != nil {
		globalEnqueueCancel()
	}
	globalEnqueueCtx, globalEnqueueCancel = context.WithCancel(context.Background())
}

func ensureEnqueueContextLocked() {
	if globalEnqueueCtx == nil || globalEnqueueCancel == nil {
		globalEnqueueCtx, globalEnqueueCancel = context.WithCancel(context.Background())
	}
}

func currentEnqueueContext() context.Context {
	globalEnqueueMu.Lock()
	defer globalEnqueueMu.Unlock()
	ensureEnqueueContextLocked()
	return globalEnqueueCtx
}

func currentEnqueueCancel() context.CancelFunc {
	globalEnqueueMu.Lock()
	defer globalEnqueueMu.Unlock()
	ensureEnqueueContextLocked()
	return globalEnqueueCancel
}

func cancelGlobalEnqueue() {
	globalEnqueueMu.Lock()
	cancel := globalEnqueueCancel
	globalEnqueueMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func takeLifecycleCleanup() func() {
	globalLifecycleMu.Lock()
	defer globalLifecycleMu.Unlock()
	cleanup := GlobalLifecycleCleanup
	GlobalLifecycleCleanup = nil
	return cleanup
}

func currentPoolConfigs() []types.DownloadConfig {
	if GlobalPool == nil {
		return nil
	}
	return GlobalPool.GetAll()
}

func lifecycleForLocalService(service core.DownloadService) (*processing.LifecycleManager, error) {
	lifecycle := currentLifecycle()
	if service == nil || GlobalService == nil || service != GlobalService {
		return lifecycle, nil
	}
	return ensureLocalLifecycle(GlobalService, currentPoolConfigs)
}

func ensureGlobalLocalServiceAndLifecycle() error {
	if GlobalService == nil {
		localService := core.NewLocalDownloadServiceWithInput(GlobalPool, GlobalProgressCh)
		GlobalService = localService

		lifecycle, err := ensureLocalLifecycle(localService, currentPoolConfigs)
		if err != nil {
			return err
		}

		lifecycle.SetEngineHooks(processing.EngineHooks{
			Pause:        GlobalPool.Pause,
			Resume:       GlobalPool.Resume,
			GetStatus:    GlobalPool.GetStatus,
			AddConfig:    GlobalPool.Add,
			PublishEvent: localService.Publish,
		})

		localService.SetLifecycleHooks(lifecycle.Pause, lifecycle.Resume, lifecycle.ResumeBatch)
	} else {
		_, err := ensureLocalLifecycle(GlobalService, currentPoolConfigs)
		return err
	}
	return nil
}

func publishSystemLog(message string) {
	if GlobalService != nil {
		_ = GlobalService.Publish(events.SystemLogMsg{Message: message})
		return
	}
	fmt.Fprintln(os.Stderr, message)
}

func recordPreflightDownloadError(url, outPath string, err error) {
	if err == nil || strings.TrimSpace(url) == "" {
		return
	}

	filename := strings.TrimSpace(processing.InferFilenameFromURL(url))
	destPath := ""
	if filename != "" && strings.TrimSpace(outPath) != "" {
		destPath = filepath.Join(outPath, filename)
	}

	entry := types.DownloadEntry{
		ID:       uuid.New().String(),
		URL:      url,
		URLHash:  state.URLHash(url),
		DestPath: destPath,
		Filename: filename,
		Status:   "error",
	}
	if addErr := state.AddToMasterList(entry); addErr != nil {
		utils.Debug("Failed to persist preflight download error for %s: %v", url, addErr)
	}
	if GlobalService != nil {
		_ = GlobalService.Publish(events.DownloadErrorMsg{
			DownloadID: entry.ID,
			Filename:   filename,
			DestPath:   destPath,
			Err:        err,
		})
	}
}

func ensureLocalLifecycle(service core.DownloadService, getAll func() []types.DownloadConfig) (*processing.LifecycleManager, error) {
	globalLifecycleMu.Lock()
	defer globalLifecycleMu.Unlock()

	if GlobalLifecycle == nil {
		GlobalLifecycle = newLocalLifecycleManager(service, getAll)
	}
	if GlobalLifecycleCleanup == nil {
		cleanup, err := startLifecycleEventWorker(service, GlobalLifecycle)
		if err != nil {
			return nil, err
		}
		GlobalLifecycleCleanup = cleanup
	}
	return GlobalLifecycle, nil
}

func isExplicitOutputPath(outPath, defaultDir string) bool {
	return utils.EnsureAbsPath(strings.TrimSpace(outPath)) != utils.EnsureAbsPath(strings.TrimSpace(defaultDir))
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:     "surge [url]...",
	Short:   "Blazing fast TUI download manager built in Go for power users",
	Long:    `Surge is a blazing fast TUI download manager built in Go for power users. Find more info here: https://github.com/surge-downloader/surge`,
	Version: Version,
	Args:    cobra.ArbitraryArgs,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set global verbose mode
		utils.SetVerbose(verbose)

		// Initialize Global Progress Channel
		GlobalProgressCh = make(chan any, 100)

		// Initialize Global Worker Pool
		globalSettings = getSettings()
		GlobalPool = download.NewWorkerPool(GlobalProgressCh, globalSettings.Network.MaxConcurrentDownloads)
	},
	Run: func(cmd *cobra.Command, args []string) {
		if hostTarget := resolveHostTarget(); hostTarget != "" {
			if len(args) > 0 {
				fmt.Fprintln(os.Stderr, "Error: URLs cannot be passed when using --host. Use 'surge add <url>' after connecting.")
				os.Exit(1)
			}
			connectAndRunTUI(cmd, hostTarget)
			return
		}

		// Attempt to acquire lock
		isMaster, err := AcquireLock()
		if err != nil {
			fmt.Printf("Error acquiring lock: %v\n", err)
			os.Exit(1)
		}

		if !isMaster {
			fmt.Fprintln(os.Stderr, "Error: Surge is already running.")
			fmt.Fprintln(os.Stderr, "Use 'surge add <url>' to add a download to the active instance.")
			os.Exit(1)
		}
		defer func() {
			if err := ReleaseLock(); err != nil {
				utils.Debug("Error releasing lock: %v", err)
			}
		}()

		mustInitializeGlobalState()
		resetGlobalEnqueueContext()

		startupIntegrityMessage = runStartupIntegrityCheck()

		if err := ensureGlobalLocalServiceAndLifecycle(); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating lifecycle event stream: %v\n", err)
			os.Exit(1)
		}

		portFlag, _ := cmd.Flags().GetInt("port")
		batchFile, _ := cmd.Flags().GetString("batch")
		outputDir, _ := cmd.Flags().GetString("output")
		noResume, _ := cmd.Flags().GetBool("no-resume")
		exitWhenDone, _ := cmd.Flags().GetBool("exit-when-done")

		port, listener, err := bindServerListener(portFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// Save port for browser extension AND CLI discovery
		saveActivePort(port)
		defer removeActivePort()

		// Start HTTP server in background (reuse the listener)
		go startHTTPServer(listener, port, outputDir, GlobalService, "")

		// Queue initial downloads if any
		atomic.AddInt32(&pendingEnqueue, 1)
		go func() {
			defer atomic.AddInt32(&pendingEnqueue, -1)
			var urls []string
			urls = append(urls, args...)

			if batchFile != "" {
				fileURLs, err := utils.ReadURLsFromFile(batchFile)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error reading batch file: %v\n", err)
				} else {
					urls = append(urls, fileURLs...)
				}
			}

			if len(urls) > 0 {
				resolvedOutputDir := resolveClientOutputPath(outputDir)
				processDownloads(urls, resolvedOutputDir, 0) // 0 port = internal direct add
			}
		}()

		// Start TUI (default mode)
		startTUI(port, exitWhenDone, noResume)
	},
}

func runStartupIntegrityCheck() string {
	// Normalize downloads stuck in "downloading" status from a prior crash/kill.
	// This must happen before ValidateIntegrity so the newly-paused entries
	// are included in the integrity check and eligible for auto-resume.
	if normalized, err := state.NormalizeStaleDownloads(); err != nil {
		msg := fmt.Sprintf("Startup: failed to normalize stale downloads: %v", err)
		utils.Debug("%s", msg)
	} else if normalized > 0 {
		utils.Debug("Startup: normalized %d stale downloading entries to paused", normalized)
	}

	// Validate integrity of paused/queued downloads before auto-resume.
	// This removes entries whose .surge files are missing/tampered and
	// also cleans orphan .surge files that no longer have DB entries.
	if removed, err := state.ValidateIntegrity(); err != nil {
		msg := fmt.Sprintf("Startup integrity check failed: %v", err)
		return msg
	} else if removed > 0 {
		msg := fmt.Sprintf("Startup integrity check: removed %d corrupted/orphaned downloads", removed)
		return msg
	}
	msg := "Startup integrity check: no issues found"
	utils.Debug("%s", msg)
	return msg
}

// startTUI initializes and runs the TUI program
func startTUI(port int, exitWhenDone bool, noResume bool) {
	// Initialize TUI
	// GlobalService and GlobalProgressCh are already initialized in PersistentPreRun or Run

	m := tui.InitialRootModel(port, Version, GlobalService, currentLifecycle(), noResume)
	m = m.WithEnqueueContext(currentEnqueueContext(), currentEnqueueCancel())
	m.ServerHost = serverBindHost
	if m.ServerHost == "" {
		m.ServerHost = "127.0.0.1"
	}
	m.IsRemote = false

	p := tea.NewProgram(m, tea.WithAltScreen())
	serverProgram = p // Save reference for HTTP handler

	// Get event stream from service
	stream, cleanup, err := GlobalService.StreamEvents(context.Background())
	if err != nil {
		_ = executeGlobalShutdown("tui: stream init failed")
		fmt.Printf("Error getting event stream: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	// Background listener for progress events
	go func() {
		for msg := range stream {
			p.Send(msg)
		}
	}()

	if startupIntegrityMessage != "" && GlobalService != nil {
		_ = GlobalService.Publish(events.SystemLogMsg{
			Message: startupIntegrityMessage,
		})
		startupIntegrityMessage = ""
	}

	// Exit-when-done checker for TUI
	if exitWhenDone {
		go func() {
			// Wait a bit for initial downloads to be queued
			time.Sleep(3 * time.Second)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if atomic.LoadInt32(&pendingEnqueue) == 0 && GlobalPool != nil && GlobalPool.ActiveCount() == 0 {
					// Send quit message to TUI
					p.Send(tea.Quit())
					return
				}
			}
		}()
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigChan)

	stopSignalListener := make(chan struct{})
	defer close(stopSignalListener)

	go func() {
		select {
		case sig := <-sigChan:
			_ = executeGlobalShutdown(fmt.Sprintf("tui signal: %s", sig))
			p.Send(tea.Quit())
		case <-stopSignalListener:
			return
		}
	}()

	// Run TUI
	if _, err := p.Run(); err != nil {
		_ = executeGlobalShutdown("tui: p.Run failed")
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
	_ = executeGlobalShutdown("tui: program exited")
}

const serverBindHost = "0.0.0.0"

// StartHeadlessConsumer starts a goroutine to consume progress messages and log to stdout
func StartHeadlessConsumer() {
	go func() {
		if GlobalService == nil {
			return
		}
		stream, cleanup, err := GlobalService.StreamEvents(context.Background())
		if err != nil {
			utils.Debug("Failed to start event stream: %v", err)
			return
		}
		defer cleanup()

		for msg := range stream {
			switch m := msg.(type) {
			case events.DownloadStartedMsg:
				fmt.Printf("Started: %s [%s]\n", m.Filename, truncateID(m.DownloadID))
			case events.DownloadCompleteMsg:
				atomic.AddInt32(&activeDownloads, -1)
				fmt.Printf("Completed: %s [%s] (in %s)\n", m.Filename, truncateID(m.DownloadID), m.Elapsed)
			case events.DownloadErrorMsg:
				atomic.AddInt32(&activeDownloads, -1)
				fmt.Printf("Error: %s [%s]: %v\n", m.Filename, truncateID(m.DownloadID), m.Err)
			case events.DownloadQueuedMsg:
				fmt.Printf("Queued: %s [%s]\n", m.Filename, truncateID(m.DownloadID))
			case events.DownloadPausedMsg:
				fmt.Printf("Paused: %s [%s]\n", m.Filename, truncateID(m.DownloadID))
			case events.DownloadResumedMsg:
				fmt.Printf("Resumed: %s [%s]\n", m.Filename, truncateID(m.DownloadID))
			case events.DownloadRemovedMsg:
				fmt.Printf("Removed: %s [%s]\n", m.Filename, truncateID(m.DownloadID))
			}
		}
	}()
}

// truncateID shortens a UUID to its first 8 characters for display
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// findAvailablePort tries ports starting from 'start' until one is available
func findAvailablePort(start int) (int, net.Listener) {
	bindHost := serverBindHost
	for port := start; port < start+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bindHost, port))
		if err == nil {
			return port, ln
		}
	}
	return 0, nil
}

func bindServerListener(portFlag int) (int, net.Listener, error) {
	bindHost := serverBindHost
	if portFlag > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bindHost, portFlag))
		if err != nil {
			return 0, nil, fmt.Errorf("could not bind to port %d: %w", portFlag, err)
		}
		return portFlag, ln, nil
	}
	port, ln := findAvailablePort(1700)
	if ln == nil {
		return 0, nil, fmt.Errorf("could not find available port")
	}
	return port, ln, nil
}

// saveActivePort writes the active port to ~/.surge/port for extension discovery
func saveActivePort(port int) {
	portFile := filepath.Join(config.GetRuntimeDir(), "port")
	if err := os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0o644); err != nil {
		utils.Debug("Error writing port file: %v", err)
	}
	utils.Debug("HTTP server listening on port %d", port)
}

// removeActivePort cleans up the port file on exit
func removeActivePort() {
	portFile := filepath.Join(config.GetRuntimeDir(), "port")
	if err := os.Remove(portFile); err != nil && !os.IsNotExist(err) {
		utils.Debug("Error removing port file: %v", err)
	}
}

// startHTTPServer starts the HTTP server using an existing listener
func startHTTPServer(ln net.Listener, port int, defaultOutputDir string, service core.DownloadService, tokenOverride string) {
	authToken := strings.TrimSpace(tokenOverride)
	if authToken == "" {
		authToken = ensureAuthToken()
	} else {
		persistAuthToken(authToken)
	}

	mux := http.NewServeMux()
	registerHTTPRoutes(mux, port, defaultOutputDir, service)

	// Wrap mux with Auth and CORS (CORS outermost to ensure 401/403 include headers)
	handler := corsMiddleware(authMiddleware(authToken, mux))

	server := &http.Server{Handler: handler}
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		utils.Debug("HTTP server error: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS, PUT, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Access-Control-Allow-Private-Network")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow health check without auth
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Allow OPTIONS for CORS preflight
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		// Check for Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			if strings.HasPrefix(authHeader, "Bearer ") {
				providedToken := strings.TrimPrefix(authHeader, "Bearer ")
				if len(providedToken) == len(token) && subtle.ConstantTimeCompare([]byte(providedToken), []byte(token)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

func ensureAuthToken() string {
	stateTokenFile := filepath.Join(config.GetStateDir(), "token")
	if token, err := readTokenFromFile(stateTokenFile); err == nil {
		return token
	}

	token := uuid.New().String()
	if err := writeTokenToFile(stateTokenFile, token); err != nil {
		utils.Debug("Failed to write token file in state dir: %v", err)
	}
	return token
}

func persistAuthToken(token string) {
	stateTokenFile := filepath.Join(config.GetStateDir(), "token")

	if err := writeTokenToFile(stateTokenFile, token); err != nil {
		utils.Debug("Failed to write token file in state dir: %v", err)
	}
}

func readTokenFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("empty token file: %s", path)
	}
	return token, nil
}

func writeTokenToFile(path string, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token), 0o600)
}

// DownloadRequest represents a download request from the browser extension
type DownloadRequest struct {
	URL                  string            `json:"url"`
	Filename             string            `json:"filename,omitempty"`
	Path                 string            `json:"path,omitempty"`
	RelativeToDefaultDir bool              `json:"relative_to_default_dir,omitempty"`
	Mirrors              []string          `json:"mirrors,omitempty"`
	SkipApproval         bool              `json:"skip_approval,omitempty"` // Extension validated request, skip TUI prompt
	Headers              map[string]string `json:"headers,omitempty"`       // Custom HTTP headers from browser (cookies, auth, etc.)
	IsExplicitCategory   bool              `json:"is_explicit_category,omitempty"`
}

func handleDownload(w http.ResponseWriter, r *http.Request, defaultOutputDir string, service core.DownloadService) {
	// GET request to query status
	if r.Method == http.MethodGet {
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing id parameter", http.StatusBadRequest)
			return
		}

		if service == nil {
			http.Error(w, "Service unavailable", http.StatusInternalServerError)
			return
		}

		status, err := service.GetStatus(id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		writeJSONResponse(w, http.StatusOK, status)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	settings := getSettings()

	var req DownloadRequest
	if err := decodeJSONBody(r, &req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	if strings.Contains(req.Path, "..") || strings.Contains(req.Filename, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	if strings.Contains(req.Filename, "/") || strings.Contains(req.Filename, "\\") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	utils.Debug("Received download request: URL=%s, Path=%s", req.URL, req.Path)

	if service == nil {
		http.Error(w, "Service unavailable", http.StatusInternalServerError)
		return
	}

	// Prepare output path
	outPath := resolveOutputDir(req.Path, req.RelativeToDefaultDir, defaultOutputDir, settings)

	// Enforce absolute path to ensure resume works even if CWD changes
	outPath = utils.EnsureAbsPath(outPath)

	// Check settings for extension prompt and duplicates
	// Logic modified to distinguish between ACTIVE (corruption risk) and COMPLETED (overwrite safe)
	isDuplicate := false
	isActive := false

	urlForAdd := req.URL
	mirrorsForAdd := req.Mirrors
	if len(mirrorsForAdd) == 0 && strings.Contains(req.URL, ",") {
		urlForAdd, mirrorsForAdd = ParseURLArg(req.URL)
	}

	activeDownloadsFunc := func() map[string]*types.DownloadConfig {
		active := make(map[string]*types.DownloadConfig)
		for _, cfg := range GlobalPool.GetAll() {
			c := cfg // create copy
			active[c.ID] = &c
		}
		return active
	}
	dupResult := processing.CheckForDuplicate(urlForAdd, settings, activeDownloadsFunc)
	if dupResult != nil {
		isDuplicate = dupResult.Exists
		isActive = dupResult.IsActive
	}

	utils.Debug("Download request: URL=%s, SkipApproval=%v, isDuplicate=%v, isActive=%v", urlForAdd, req.SkipApproval, isDuplicate, isActive)

	// EXTENSION VETTING SHORTCUT:
	// If SkipApproval is true, we trust the extension completely.
	// The backend will auto-rename duplicate files, so no need to reject.
	if req.SkipApproval {
		// Trust extension -> Skip all prompting logic, proceed to download
		utils.Debug("Extension request: skipping all prompts, proceeding with download")
	} else {
		// Logic for prompting:
		// 1. If ExtensionPrompt is enabled
		// 2. OR if WarnOnDuplicate is enabled AND it is a duplicate
		shouldPrompt := settings.General.ExtensionPrompt || (settings.General.WarnOnDuplicate && isDuplicate)

		// Only prompt if we have a UI running (serverProgram != nil)
		if shouldPrompt {
			if serverProgram != nil {
				utils.Debug("Requesting TUI confirmation for: %s (Duplicate: %v)", req.URL, isDuplicate)

				// Send request to TUI
				downloadID := uuid.New().String()
				if err := service.Publish(events.DownloadRequestMsg{
					ID:       downloadID,
					URL:      urlForAdd,
					Filename: req.Filename,
					Path:     outPath, // Use the path we resolved (default or requested)
					Mirrors:  mirrorsForAdd,
					Headers:  req.Headers,
				}); err != nil {
					http.Error(w, "Failed to notify TUI: "+err.Error(), http.StatusInternalServerError)
					return
				}

				// Return 202 Accepted to indicate it's pending approval
				writeJSONResponse(w, http.StatusAccepted, map[string]string{
					"status":  "pending_approval",
					"message": "Download request sent to TUI for confirmation",
					"id":      downloadID, // ID might change if user modifies it, but useful for tracking
				})
				return
			} else {
				// Headless mode check
				writeJSONResponse(w, http.StatusConflict, map[string]string{
					"status":  "error",
					"message": "Download rejected: Duplicate download or approval required (Headless mode)",
				})
				return
			}
		}
	}

	lifecycle, err := lifecycleForLocalService(service)
	if err != nil {
		http.Error(w, "Failed to initialize lifecycle manager: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var newID string
	if lifecycle != nil {
		newID, err = lifecycle.Enqueue(r.Context(), &processing.DownloadRequest{
			URL:                urlForAdd,
			Filename:           req.Filename,
			Path:               outPath,
			Mirrors:            mirrorsForAdd,
			Headers:            req.Headers,
			IsExplicitCategory: req.IsExplicitCategory,
			SkipApproval:       req.SkipApproval,
		})
	} else {
		newID, err = service.Add(urlForAdd, outPath, req.Filename, mirrorsForAdd, req.Headers, req.IsExplicitCategory, 0, false)
	}
	if err != nil {
		recordPreflightDownloadError(urlForAdd, outPath, err)
		http.Error(w, "Failed to add download: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Increment active downloads counter
	atomic.AddInt32(&activeDownloads, 1)

	writeJSONResponse(w, http.StatusOK, map[string]string{
		"status":  "queued",
		"message": "Download queued successfully",
		"id":      newID,
	})
}

// processDownloads handles the logic of adding downloads either to local pool or remote server
// Returns the number of successfully added downloads
func processDownloads(urls []string, outputDir string, port int) int {
	successCount := 0

	// If port > 0, we are sending to a remote server
	if port > 0 {
		baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
		token := resolveLocalToken()
		for _, arg := range urls {
			url, mirrors := ParseURLArg(arg)
			if url == "" {
				continue
			}
			err := sendToServer(url, mirrors, outputDir, baseURL, token)
			if err != nil {
				fmt.Printf("Error adding %s: %v\n", url, err)
			} else {
				successCount++
			}
		}
		return successCount
	}

	// Internal add (TUI or Headless mode)
	if GlobalService == nil {
		fmt.Fprintln(os.Stderr, "Error: GlobalService not initialized")
		return 0
	}

	settings := getSettings()

	lifecycle, err := lifecycleForLocalService(GlobalService)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: unable to initialize lifecycle manager:", err)
		return 0
	}

	for _, arg := range urls {
		// Validation
		if arg == "" {
			continue
		}

		url, mirrors := ParseURLArg(arg)
		if url == "" {
			continue
		}

		// Prepare output path
		outPath := resolveOutputDir(outputDir, false, "", settings)
		outPath = utils.EnsureAbsPath(outPath)

		// Check for duplicates/extensions if we are in TUI mode (serverProgram != nil)
		// For headless/root direct add, we might skip prompt or auto-approve?
		// For now, let's just add directly if headless, or prompt if TUI is up.

		// If TUI is up (serverProgram != nil), we might want to send a request msg?
		// But processDownloads is called from QUEUE init routine, primarily for CLI args.
		// If CLI args provided, user probably wants them added immediately.

		// CLI explicit arg means we do not auto-route when user provided an explicit output path.
		isExplicit := isExplicitOutputPath(outPath, settings.General.DefaultDownloadDir)
		if lifecycle == nil {
			err := fmt.Errorf("lifecycle manager unavailable")
			recordPreflightDownloadError(url, outPath, err)
			publishSystemLog(fmt.Sprintf("Error adding %s: %v", url, err))
			continue
		}

		_, err := lifecycle.Enqueue(currentEnqueueContext(), &processing.DownloadRequest{
			URL:                url,
			Path:               outPath,
			Mirrors:            mirrors,
			IsExplicitCategory: isExplicit,
		})
		if err != nil {
			recordPreflightDownloadError(url, outPath, err)
			publishSystemLog(fmt.Sprintf("Error adding %s: %v", url, err))
			continue
		}
		atomic.AddInt32(&activeDownloads, 1)
		successCount++
	}
	return successCount
}

func resolveOutputDir(reqPath string, relativeToDefaultDir bool, defaultOutputDir string, settings *config.Settings) string {
	outPath := reqPath

	if relativeToDefaultDir && reqPath != "" {
		baseDir := settings.General.DefaultDownloadDir
		if baseDir == "" {
			baseDir = defaultOutputDir
		}
		if baseDir == "" {
			baseDir = "."
		}
		outPath = filepath.Join(baseDir, reqPath)
	} else if outPath == "" {
		if defaultOutputDir != "" {
			outPath = defaultOutputDir
		} else if settings.General.DefaultDownloadDir != "" {
			outPath = settings.General.DefaultDownloadDir
		} else {
			outPath = "."
		}
	}

	_ = os.MkdirAll(outPath, 0o755)
	return outPath
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().StringVar(&globalHost, "host", "", "Server host to connect/control (or set SURGE_HOST), e.g. 127.0.0.1:1700")
	rootCmd.PersistentFlags().StringVar(&globalToken, "token", "", "Bearer token (or set SURGE_TOKEN)")
	rootCmd.Flags().StringP("batch", "b", "", "File containing URLs to download (one per line)")
	rootCmd.Flags().IntP("port", "p", 0, "Port to listen on (default: 8080 or first available)")
	rootCmd.Flags().StringP("output", "o", "", "Output directory (defaults to current working directory)")
	rootCmd.Flags().Bool("no-resume", false, "Do not auto-resume paused downloads on startup")
	rootCmd.Flags().Bool("exit-when-done", false, "Exit when all downloads complete")
	rootCmd.SetVersionTemplate("Surge v{{.Version}}\n")
	rootCmd.Version = Version
}

func mustInitializeGlobalState() {
	if err := initializeGlobalState(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// initializeGlobalState sets up the environment and configures the engine state and logging
func initializeGlobalState() error {

	stateDir := config.GetStateDir()
	logsDir := config.GetLogsDir()
	stateDBPath := filepath.Join(stateDir, "surge.db")

	// Ensure directories exist
	_ = os.MkdirAll(stateDir, 0o755)
	_ = os.MkdirAll(logsDir, 0o755)

	// Config engine state
	state.Configure(stateDBPath)

	// Config logging
	utils.ConfigureDebug(logsDir)

	// Clean up old logs
	retention := getSettings().General.LogRetentionCount
	utils.CleanupLogs(retention)
	return nil
}

func getSettings() *config.Settings {
	if globalSettings != nil {
		return globalSettings
	}
	settings, err := config.LoadSettings()
	if err != nil {
		return config.DefaultSettings()
	}
	return settings
}

func resumePausedDownloads() {
	settings := getSettings()

	pausedEntries, err := state.LoadPausedDownloads()
	if err != nil {
		return
	}

	for _, entry := range pausedEntries {
		// If entry is explicitly queued, we should start it regardless of AutoResume setting
		// If entry is paused, we only start it if AutoResume is enabled
		if entry.Status == "paused" && !settings.General.AutoResume {
			continue
		}
		if GlobalService == nil || entry.ID == "" {
			continue
		}
		if err := GlobalService.Resume(entry.ID); err == nil {
			atomic.AddInt32(&activeDownloads, 1)
		}
	}
}
