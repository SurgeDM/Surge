package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/engine/types"
	"github.com/surge-downloader/surge/internal/processing"
	"github.com/surge-downloader/surge/internal/tui/colors"
	"github.com/surge-downloader/surge/internal/version"
)

type UIState int // Defines UIState as int to be used in rootModel

const (
	DashboardState             UIState = iota // DashboardState is 0 increments after each line
	InputState                                // InputState is 1
	DetailState                               // DetailState is 2
	FilePickerState                           // FilePickerState is 3
	HistoryState                              // HistoryState is 4
	DuplicateWarningState                     // DuplicateWarningState is 5
	SearchState                               // SearchState is 6
	SettingsState                             // SettingsState is 7
	ExtensionConfirmationState                // ExtensionConfirmationState is 8
	BatchFilePickerState                      // BatchFilePickerState is 9
	BatchConfirmState                         // BatchConfirmState is 10
	UpdateAvailableState                      // UpdateAvailableState is 11
	URLUpdateState                            // URLUpdateState is 12
	CategoryManagerState                      // CategoryManagerState is 13
	QuitConfirmState                          // QuitConfirmState is 14
)

const (
	TabQueued = 0
	TabActive = 1
	TabDone   = 2
)

type DownloadModel struct {
	ID            string
	URL           string
	Filename      string
	FilenameLower string
	Destination   string // Full path to the destination file
	Total         int64
	Downloaded    int64
	Speed         float64
	Connections   int

	StartTime time.Time
	Elapsed   time.Duration
	lastETA   time.Duration // EMA-smoothed ETA for UI stability

	progress progress.Model

	// Unified architecture: View Model updated by events
	// No direct state access or polling reporter
	state *types.ProgressState // Keep for now if needed for details view, but mostly passive

	done     bool
	err      error
	paused   bool
	pausing  bool // UI state: transitioning to pause
	resuming bool // UI state: waiting for async resume
}

type RootModel struct {
	downloads    []*DownloadModel
	width        int
	height       int
	state        UIState
	activeTab    int // 0=Queued, 1=Active, 2=Done
	inputs       []textinput.Model
	focusedInput int
	// Service Interface
	// Core
	Service      core.DownloadService
	Orchestrator *processing.LifecycleManager

	// File picker for directory selection
	filepicker filepicker.Model

	// Bubbles help component
	help help.Model

	// Bubbles list component for download listing
	list list.Model

	PWD string

	// History view
	historyEntries []types.DownloadEntry
	historyCursor  int

	// Duplicate detection
	pendingURL           string // URL pending confirmation
	pendingPath          string // Path pending confirmation
	pendingIsDefaultPath bool
	pendingFilename      string   // Filename pending confirmation
	pendingMirrors       []string // Mirrors pending confirmation
	pendingHeaders       map[string]string
	duplicateInfo        string // Info about the duplicate

	// Graph Data
	SpeedHistory           []float64 // Stores the last ~60 ticks of speed data
	lastSpeedHistoryUpdate time.Time // Last time SpeedHistory was updated (for 0.5s sampling)

	// Notification log system
	logViewport viewport.Model // Scrollable log viewport
	logEntries  []string       // Log entries for download events
	logFocused  bool           // Whether the log viewport is focused

	// Settings
	Settings              *config.Settings // Application settings
	SettingsActiveTab     int              // Active category tab (0-3)
	SettingsSelectedRow   int              // Selected setting within current tab
	SettingsIsEditing     bool             // Whether currently editing a value
	SettingsInput         textinput.Model  // Input for editing string/int values
	SettingsFileBrowsing  bool             // Whether browsing for a directory
	ExtensionFileBrowsing bool             // Whether browsing for extension prompt path

	// Selection persistence
	SelectedDownloadID string // ID of the currently selected download
	ManualTabSwitch    bool   // Whether the last tab switch was manual

	// Search functionality
	searchInput  textinput.Model // Text input for search
	searchActive bool            // Whether search mode is active
	searchQuery  string          // Current search query

	// Batch import
	pendingBatchURLs []string // URLs pending batch import
	batchFilePath    string   // Path to the batch file

	// URL Refresh
	urlUpdateInput textinput.Model // Text input for updating URL

	// Category manager
	categoryFilter     string             // Dashboard filter ("" = all)
	catMgrCursor       int                // Selected category index
	catMgrEditing      bool               // Whether editing a category
	catMgrEditField    int                // 0=Name, 1=Description, 2=Pattern, 3=Path
	catMgrInputs       [4]textinput.Model // Inputs for Name, Description, Pattern, Path
	catMgrIsNew        bool               // Whether adding a new category
	catMgrFileBrowsing bool               // Whether browsing for a category path

	// Quit confirm button focus (0 = Yep!, 1 = Nope)
	quitConfirmFocused int

	// Keybindings
	keys KeyMap

	// Server port for display
	ServerPort int
	ServerHost string
	IsRemote   bool

	// Update check
	UpdateInfo     *version.UpdateInfo // Update information (nil if no update available)
	CurrentVersion string              // Current version of Surge

	InitialDarkBackground bool // Captured at startup for "System" theme

	logoCache string // Cached logo with gradient applied

	enqueueCtx    context.Context
	cancelEnqueue context.CancelFunc
	shuttingDown  bool
}

// NewDownloadModel creates a new download model
func NewDownloadModel(id string, url string, filename string, total int64) *DownloadModel {
	// Create dummy state container for compatibility if needed
	state := types.NewProgressState(id, total)
	return &DownloadModel{
		ID:            id,
		URL:           url,
		Filename:      filename,
		FilenameLower: strings.ToLower(filename),
		Total:         total,
		StartTime:     time.Now(),
		progress:      progress.New(progress.WithSpringOptions(0.5, 0.1)),
		state:         state,
	}
}

func InitialRootModel(serverPort int, currentVersion string, service core.DownloadService, orchestrator *processing.LifecycleManager, noResume bool) RootModel {
	initialDarkBackground := lipgloss.HasDarkBackground(os.Stdin, os.Stdout)

	// Initialize inputs
	urlInput := textinput.New()
	urlInput.Placeholder = "https://example.com/file.zip"
	urlInput.Focus()
	urlInput.SetWidth(InputWidth)
	urlInput.Prompt = ""

	pathInput := textinput.New()
	pathInput.Placeholder = "."
	pathInput.SetWidth(InputWidth)
	pathInput.Prompt = ""
	pathInput.SetValue(".")

	filenameInput := textinput.New()
	filenameInput.Placeholder = "(auto-detect)"
	filenameInput.SetWidth(InputWidth)
	filenameInput.Prompt = ""

	mirrorsInput := textinput.New()
	mirrorsInput.Placeholder = "http://mirror1.com, http://mirror2.com"
	mirrorsInput.SetWidth(InputWidth)
	mirrorsInput.Prompt = ""

	pwd, _ := os.Getwd()

	// Initialize file picker for directory selection - default to Downloads folder
	homeDir, _ := os.UserHomeDir()
	downloadsDir := filepath.Join(homeDir, "Downloads")
	fp := filepicker.New()
	fp.CurrentDirectory = downloadsDir
	fp.DirAllowed = true
	fp.FileAllowed = false
	fp.ShowHidden = false
	fp.ShowSize = true
	fp.ShowPermissions = true
	fp.SetHeight(FilePickerHeight)

	// Load settings for auto resume
	settings, _ := config.LoadSettings()
	if settings == nil {
		settings = config.DefaultSettings()
	}

	// Override AutoResume if CLI flag provided
	if noResume {
		settings.General.AutoResume = false
	}

	applyColorModeForTheme(settings.General.Theme, initialDarkBackground)

	// Load paused downloads from master list (now uses global config directory)
	var downloads []*DownloadModel
	// Note: With Service abstraction, we might want to let the Service handle loading.
	// But LocalDownloadService's List() calls state.ListAllDownloads().
	// For TUI initialization, we should probably call Service.List() to populate the model.
	// However, Service.List() returns []DownloadStatus, which we need to convert to []*DownloadModel.

	// Let's use service.List() if available
	if service != nil {
		statuses, err := service.List()
		if err == nil {
			for _, s := range statuses {
				dm := NewDownloadModel(s.ID, s.URL, s.Filename, s.TotalSize)
				dm.Downloaded = s.Downloaded
				if s.DestPath != "" {
					dm.Destination = s.DestPath
				} else {
					dm.Destination = s.Filename // Fallback
				}
				// Status mapping
				switch s.Status {
				case "completed":
					dm.done = true
					dm.progress.SetPercent(1.0)
				case "error":
					dm.done = true
				case "pausing":
					dm.pausing = true
				case "paused":
					if settings.General.AutoResume {
						dm.resuming = true
						dm.paused = true // Will update when resume event received
					} else {
						dm.paused = true
					}
				case "queued":
					// Always resume queued items
					dm.resuming = true
					dm.paused = true // Will update when resume event received
				}

				if s.TotalSize > 0 {
					dm.progress.SetPercent(s.Progress / 100.0)
				}
				if s.AvgSpeed > 0 {
					dm.Speed = s.AvgSpeed
				} else if s.Speed > 0 {
					dm.Speed = s.Speed * float64(config.MB)
				}
				if s.Status == "completed" && s.TimeTaken > 0 {
					dm.Elapsed = time.Duration(s.TimeTaken) * time.Millisecond
				}

				downloads = append(downloads, dm)
			}
		}
	}

	// Initialize the download list
	downloadList := NewDownloadList(80, 20) // Default size, will be resized on WindowSizeMsg

	// Initialize help
	helpModel := help.New()
	helpModel.Styles.ShortKey = lipgloss.NewStyle().Foreground(colors.LightGray)
	helpModel.Styles.ShortDesc = lipgloss.NewStyle().Foreground(colors.Gray)

	// Initialize settings input for editing
	settingsInput := textinput.New()
	settingsInput.SetWidth(40)
	settingsInput.Prompt = ""

	// Initialize search input
	searchInput := textinput.New()
	searchInput.Placeholder = "Type to search..."
	searchInput.SetWidth(30)
	searchInput.Prompt = ""

	// Initialize URL update input
	urlUpdateInput := textinput.New()
	urlUpdateInput.Placeholder = "https://example.com/newlink.zip"
	urlUpdateInput.SetWidth(InputWidth)
	urlUpdateInput.Prompt = ""

	// Initialize Category Manager inputs
	catNameInput := textinput.New()
	catNameInput.Placeholder = "Videos"
	catNameInput.SetWidth(30)
	catNameInput.Prompt = ""

	catDescInput := textinput.New()
	catDescInput.Placeholder = "Video files (.mp4, .mkv)"
	catDescInput.SetWidth(50)
	catDescInput.Prompt = ""

	catPatternInput := textinput.New()
	catPatternInput.Placeholder = "(?i)\\.(mp4|mkv)$"
	catPatternInput.SetWidth(50)
	catPatternInput.Prompt = ""

	catPathInput := textinput.New()
	catPathInput.Placeholder = "/home/user/Videos"
	catPathInput.SetWidth(50)
	catPathInput.Prompt = ""

	enqueueCtx, cancelEnqueue := context.WithCancel(context.Background())

	m := RootModel{
		downloads:             downloads,
		inputs:                []textinput.Model{urlInput, mirrorsInput, pathInput, filenameInput},
		state:                 DashboardState,
		filepicker:            fp,
		help:                  helpModel,
		list:                  downloadList,
		Service:               service,
		Orchestrator:          orchestrator,
		PWD:                   pwd,
		Settings:              settings,
		SpeedHistory:          make([]float64, GraphHistoryPoints),                          // 60 points of history (30s at 0.5s interval)
		logViewport:           viewport.New(viewport.WithWidth(40), viewport.WithHeight(5)), // Default size, will be resized
		logEntries:            make([]string, 0),
		SettingsInput:         settingsInput,
		searchInput:           searchInput,
		urlUpdateInput:        urlUpdateInput,
		catMgrInputs:          [4]textinput.Model{catNameInput, catDescInput, catPatternInput, catPathInput},
		keys:                  Keys,
		ServerPort:            serverPort,
		CurrentVersion:        currentVersion,
		InitialDarkBackground: initialDarkBackground,
		enqueueCtx:            enqueueCtx,
		cancelEnqueue:         cancelEnqueue,
	}

	m.refreshThemeCaches()

	return m
}

// WithEnqueueContext lets callers bind model-initiated probes to a process-level
// shutdown context instead of the model's default standalone context.
func (m RootModel) WithEnqueueContext(ctx context.Context, cancel context.CancelFunc) RootModel {
	if ctx == nil {
		ctx = context.Background()
	}
	if cancel == nil {
		cancel = func() {}
	}
	m.enqueueCtx = ctx
	m.cancelEnqueue = cancel
	return m
}

type ViewStats struct {
	ActiveCount     int
	QueuedCount     int
	DownloadedCount int
	TotalDownloaded int64
}

func (m RootModel) Init() tea.Cmd {
	var cmds []tea.Cmd

	// Trigger update check if not disabled in settings
	if !m.Settings.General.SkipUpdateCheck {
		cmds = append(cmds, checkForUpdateCmd(m.CurrentVersion))
	}

	// Async resume of downloads
	var resumeIDs []string
	for _, d := range m.downloads {
		if d.resuming {
			resumeIDs = append(resumeIDs, d.ID)
		}
	}

	if len(resumeIDs) > 0 && m.Service != nil {
		cmds = append(cmds, func() tea.Msg {
			errs := m.Service.ResumeBatch(resumeIDs)

			// Dispatch individual messages for UI updates
			var batch []tea.Cmd
			for i, id := range resumeIDs {
				err := errs[i]
				// Capture for closure
				currentID := id
				currentErr := err
				batch = append(batch, func() tea.Msg {
					return resumeResultMsg{id: currentID, err: currentErr}
				})
			}
			return tea.Batch(batch...)()
		})
	}

	return tea.Batch(cmds...)
}

type resumeResultMsg struct {
	id  string
	err error
}

// FindDownloadByID finds a download by its ID
func (m *RootModel) FindDownloadByID(id string) *DownloadModel {
	for _, d := range m.downloads {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// Helper to get downloads for the current tab
func (m RootModel) getFilteredDownloads() []*DownloadModel {
	var filtered []*DownloadModel
	searchLower := strings.ToLower(m.searchQuery)

	for _, d := range m.downloads {
		// Apply tab filter first
		switch m.activeTab {
		case TabQueued:
			if d.done || d.Speed > 0 {
				continue
			}
		case TabActive:
			if d.done || (d.Speed == 0 && d.Connections == 0) {
				continue
			}
		case TabDone:
			if !d.done {
				continue
			}
		}

		// Apply dashboard category filter.
		if m.categoryFilter != "" && m.Settings != nil && m.Settings.General.CategoryEnabled {
			if !m.matchesCategoryFilter(d) {
				continue
			}
		}

		// Apply search filter if query is set
		if m.searchQuery != "" {
			if !strings.Contains(d.FilenameLower, searchLower) {
				continue
			}
		}

		filtered = append(filtered, d)
	}
	return filtered
}

func (m RootModel) matchesCategoryFilter(d *DownloadModel) bool {
	filter := m.categoryFilter
	if filter == "" {
		return true
	}

	filename := strings.TrimSpace(d.Filename)
	if filename == "" || filename == "Queued" {
		if d.Destination != "" {
			if destBase := strings.TrimSpace(filepath.Base(d.Destination)); strings.Contains(destBase, ".") {
				filename = destBase
			}
		}
	}
	if filename == "" || filename == "Queued" {
		filename = processing.InferFilenameFromURL(d.URL)
	}

	cat, err := config.GetCategoryForFile(filename, m.Settings.General.Categories)
	if filter == "Uncategorized" {
		return err != nil || cat == nil
	}

	return err == nil && cat != nil && cat.Name == filter
}

// newFilepicker creates a fresh filepicker instance with consistent settings.
// This is necessary to avoid cursor desync issues that cause "index out of range"
// panics when navigating directories (especially on Windows).
// See: https://github.com/charmbracelet/bubbles/issues/864
func newFilepicker(currentDir string) filepicker.Model {
	fp := filepicker.New()
	fp.CurrentDirectory = currentDir
	fp.DirAllowed = true
	fp.FileAllowed = false
	fp.ShowHidden = false
	fp.ShowSize = true
	fp.ShowPermissions = true
	fp.SetHeight(FilePickerHeight)
	return fp
}

// ApplyTheme applies the selected theme mode
func (m *RootModel) ApplyTheme(mode int) {
	applyColorModeForTheme(mode, m.InitialDarkBackground)
	m.refreshThemeCaches()
}

func applyColorModeForTheme(mode int, initialDarkBackground bool) {
	switch mode {
	case config.ThemeAdaptive:
		colors.SetDarkMode(initialDarkBackground)
	case config.ThemeLight:
		colors.SetDarkMode(false)
	case config.ThemeDark:
		colors.SetDarkMode(true)
	default:
		colors.SetDarkMode(initialDarkBackground)
	}
}

func (m *RootModel) refreshThemeCaches() {
	rebuildStyles()
	m.help.Styles.ShortKey = lipgloss.NewStyle().Foreground(colors.LightGray)
	m.help.Styles.ShortDesc = lipgloss.NewStyle().Foreground(colors.Gray)
	applyListTheme(&m.list)
	m.logoCache = ""
}
