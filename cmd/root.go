package cmd

import (
	"context"
	"fmt"
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

// activeDownloads tracks in-flight downloads for headless/server exit logic.
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

type rootRunOptions struct {
	portFlag     int
	batchFile    string
	outputDir    string
	noResume     bool
	exitWhenDone bool
}

func readRootRunOptions(cmd *cobra.Command) rootRunOptions {
	portFlag, _ := cmd.Flags().GetInt("port")
	batchFile, _ := cmd.Flags().GetString("batch")
	outputDir, _ := cmd.Flags().GetString("output")
	noResume, _ := cmd.Flags().GetBool("no-resume")
	exitWhenDone, _ := cmd.Flags().GetBool("exit-when-done")

	return rootRunOptions{
		portFlag:     portFlag,
		batchFile:    batchFile,
		outputDir:    outputDir,
		noResume:     noResume,
		exitWhenDone: exitWhenDone,
	}
}

func maybeRunRemoteTUI(cmd *cobra.Command, args []string) bool {
	hostTarget := resolveHostTarget()
	if hostTarget == "" {
		return false
	}

	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "Error: URLs cannot be passed when using --host. Use 'surge add <url>' after connecting.")
		os.Exit(1)
	}

	connectAndRunTUI(cmd, hostTarget)
	return true
}

func acquireRootInstanceLock() func() {
	isMaster, err := AcquireLock()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error acquiring lock: %v\n", err)
		os.Exit(1)
	}

	if !isMaster {
		fmt.Fprintln(os.Stderr, "Error: Surge is already running.")
		fmt.Fprintln(os.Stderr, "Use 'surge add <url>' to add a download to the active instance.")
		os.Exit(1)
	}

	return func() {
		if err := ReleaseLock(); err != nil {
			utils.Debug("Error releasing lock: %v", err)
		}
	}
}

func initializeRootLocalRuntime() {
	mustInitializeGlobalState()
	resetGlobalEnqueueContext()

	startupIntegrityMessage = runStartupIntegrityCheck()

	if err := ensureGlobalLocalServiceAndLifecycle(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating lifecycle event stream: %v\n", err)
		os.Exit(1)
	}
}

func startRootHTTPServer(opts rootRunOptions) (int, func()) {
	port, listener, err := bindServerListener(opts.portFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	saveActivePort(port)
	go startHTTPServer(listener, port, opts.outputDir, GlobalService, "")

	return port, func() {
		removeActivePort()
	}
}

func queueInitialRootDownloads(args []string, opts rootRunOptions) {
	atomic.AddInt32(&pendingEnqueue, 1)
	go func() {
		defer atomic.AddInt32(&pendingEnqueue, -1)
		var urls []string
		urls = append(urls, args...)

		if opts.batchFile != "" {
			fileURLs, err := utils.ReadURLsFromFile(opts.batchFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading batch file: %v\n", err)
			} else {
				urls = append(urls, fileURLs...)
			}
		}

		if len(urls) > 0 {
			resolvedOutputDir := resolveClientOutputPath(opts.outputDir)
			processDownloads(urls, resolvedOutputDir, 0) // 0 port = internal direct add
		}
	}()
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

		GlobalProgressCh = make(chan any, 100)
		globalSettings = getSettings()
		GlobalPool = download.NewWorkerPool(GlobalProgressCh, globalSettings.Network.MaxConcurrentDownloads)
	},
	Run: func(cmd *cobra.Command, args []string) {
		if maybeRunRemoteTUI(cmd, args) {
			return
		}

		releaseLock := acquireRootInstanceLock()
		defer releaseLock()

		initializeRootLocalRuntime()

		opts := readRootRunOptions(cmd)
		port, cleanup := startRootHTTPServer(opts)
		defer cleanup()

		queueInitialRootDownloads(args, opts)
		startTUI(port, opts.exitWhenDone, opts.noResume)
	},
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
		fmt.Fprintf(os.Stderr, "Error getting event stream: %v\n", err)
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
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
	_ = executeGlobalShutdown("tui: program exited")
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
