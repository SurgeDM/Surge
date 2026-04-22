package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/core"
	"github.com/SurgeDM/Surge/internal/engine/types"
	"github.com/SurgeDM/Surge/internal/processing"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/SurgeDM/Surge/internal/tui/components"
	"github.com/SurgeDM/Surge/internal/version"
)

// InitializeTUI prepares global TUI state like styles and component caches.
// This should be called exactly once before any TUI elements are rendered.
func InitializeTUI() {
	InitializeStyles()
	components.InitializeStatusCache()
}

type UIState int // Defines UIState as int to be used in rootModel

const (
	DashboardState             UIState = iota // DashboardState is 0 increments after each line
	InputState                                // InputState is 1
	DetailState                               // DetailState is 2
	FilePickerState                           // FilePickerState is 3
	DuplicateWarningState                     // DuplicateWarningState is 4
	SearchState                               // SearchState is 6
	SettingsState                             // SettingsState is 7
	ExtensionConfirmationState                // ExtensionConfirmationState is 8
	BatchFilePickerState                      // BatchFilePickerState is 9
	BatchConfirmState                         // BatchConfirmState is 10
	UpdateAvailableState                      // UpdateAvailableState is 11
	URLUpdateState                            // URLUpdateState is 12
	CategoryManagerState                      // CategoryManagerState is 13
	QuitConfirmState                          // QuitConfirmState is 14
	HelpModalState                            // HelpModalState is 15
)

type FilePickerOrigin int

const (
	FilePickerOriginNone FilePickerOrigin = iota
	FilePickerOriginAdd
	FilePickerOriginSettings
	FilePickerOriginExtension
	FilePickerOriginCategory
	FilePickerOriginTheme
)

const (
	TabQueued = 0
	TabActive = 1
	TabDone   = 2
)

type DownloadModel struct {
	progress      progress.Model
	StartTime     time.Time
	err           error
	state         *types.ProgressState
	Destination   string
	ID            string
	URL           string
	Filename      string
	FilenameLower string
	lastETA       time.Duration
	Elapsed       time.Duration
	Total         int64
	Connections   int
	Speed         float64
	Downloaded    int64
	done          bool
	paused        bool
	pausing       bool
	resuming      bool
}

type RootModel struct {
	list                   list.Model
	filepicker             filepicker.Model
	lastSpeedHistoryUpdate time.Time
	Service                core.DownloadService
	enqueueCtx             context.Context
	Settings               *config.Settings
	pendingHeaders         map[string]string
	cancelEnqueue          context.CancelFunc
	Orchestrator           *processing.LifecycleManager
	UpdateInfo             *version.UpdateInfo
	help                   help.Model
	pendingPath            string
	pendingFilename        string
	batchFilePath          string
	PWD                    string
	pendingURL             string
	searchQuery            string
	ServerHost             string
	logoCache              string
	SelectedDownloadID     string
	CurrentVersion         string
	duplicateInfo          string
	filepickerOriginalPath string
	categoryFilter         string
	keys                   KeyMap
	SpeedHistory           []float64
	logEntries             []string
	downloads              []*DownloadModel
	pendingMirrors         []string
	inputs                 []textinput.Model
	pendingBatchURLs       []string
	catMgrInputs           [4]textinput.Model
	SettingsInput          textinput.Model
	urlUpdateInput         textinput.Model
	searchInput            textinput.Model
	logViewport            viewport.Model
	spinner                spinner.Model
	SettingsSelectedRow    int
	ServerPort             int
	width                  int
	height                 int
	state                  UIState
	catMgrCursor           int
	activeTab              int
	catMgrEditField        int
	SettingsActiveTab      int
	focusedInput           int
	quitConfirmFocused     int
	filepickerOrigin       FilePickerOrigin
	catMgrEditing          bool
	pendingIsDefaultPath   bool
	IsRemote               bool
	logFocused             bool
	catMgrIsNew            bool
	InitialDarkBackground  bool
	searchActive           bool
	SettingsIsEditing      bool
	ExtensionTokenCopied   bool
	shuttingDown           bool
	ManualTabSwitch        bool
}

// NewDownloadModel creates a new download model.
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
		progress: progress.New(
			progress.WithSpringOptions(0.5, 0.1),
			progress.WithColors(colors.ProgressStart(), colors.ProgressEnd()),
			progress.WithScaled(true),
		),
		state: state,
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
	applyFilepickerTheme(&fp)

	// Load settings for auto resume
	settings, _ := config.LoadSettings()
	if settings == nil {
		settings = config.DefaultSettings()
	}

	// Override AutoResume if CLI flag provided
	if noResume {
		settings.General.AutoResume = false
	}

	applyColorModeForTheme(settings.General.Theme, settings.General.ThemePath, initialDarkBackground)

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
	helpModel.Styles.ShortKey = lipgloss.NewStyle().Foreground(colors.LightGray())
	helpModel.Styles.ShortDesc = lipgloss.NewStyle().Foreground(colors.Gray())
	helpModel.Styles.FullKey = lipgloss.NewStyle().Foreground(colors.Pink())
	helpModel.Styles.FullDesc = lipgloss.NewStyle().Foreground(colors.LightGray())

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

	// A single root-level spinner provides a shared animation frame for rendering,
	// avoiding the CPU and redraw overhead of independent per-item spinners on
	// large download lists.
	s := spinner.New()
	s.Spinner = spinner.MiniDot

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
		spinner:               s,
	}

	InitAuthToken() // Cache auth token for TUI to avoid per-frame disk I/O

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

	cmds = append(cmds, m.spinner.Tick)

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

// FindDownloadByID finds a download by its ID.
func (m *RootModel) FindDownloadByID(id string) *DownloadModel {
	for _, d := range m.downloads {
		if d.ID == id {
			return d
		}
	}
	return nil
}

// Helper to get downloads for the current tab.
func (m RootModel) getFilteredDownloads() []*DownloadModel {
	var filtered []*DownloadModel
	searchLower := strings.ToLower(m.searchQuery)

	for _, d := range m.downloads {
		// Apply tab filter first
		switch m.activeTab {
		case TabQueued:
			// Queued includes paused downloads and anything not currently active or done
			if d.done || (!d.paused && !d.pausing && (d.Speed > 0 || d.Connections > 0 || d.resuming)) {
				continue
			}
		case TabActive:
			// Active excludes paused downloads and anything without current activity
			if d.done || d.paused || d.pausing || (d.Speed == 0 && d.Connections == 0 && !d.resuming) {
				continue
			}
		case TabDone:
			if !d.done {
				continue
			}
		}

		// Apply dashboard category filter.
		if m.categoryFilter != "" && m.Settings != nil && m.Settings.Categories.CategoryEnabled {
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

	cat, err := config.GetCategoryForFile(filename, m.Settings.Categories.Categories)
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

	// Re-bind Select and Open to '.' per user preference.
	// We also keep 'right' for Open to allow directory navigation.
	fp.KeyMap.Select = key.NewBinding(key.WithKeys("."))
	fp.KeyMap.Open = key.NewBinding(key.WithKeys(".", "right"))
	// Keep ESC reserved for dismissing the modal; use left/backspace/h to go up.
	fp.KeyMap.Back = key.NewBinding(key.WithKeys("h", "backspace", "left"))

	applyFilepickerTheme(&fp)

	return fp
}

// ApplyTheme applies the selected theme mode.
func (m *RootModel) ApplyTheme(mode int, path string) {
	applyColorModeForTheme(mode, path, m.InitialDarkBackground)
	m.refreshThemeCaches()
}

func applyColorModeForTheme(mode int, themePath string, initialDarkBackground bool) {
	isDark := initialDarkBackground

	switch mode {
	case config.ThemeLight:
		isDark = false
	case config.ThemeDark:
		isDark = true
	}

	colors.LoadTheme(themePath, isDark)
}

func (m *RootModel) refreshThemeCaches() {
	rebuildStyles()
	m.help.Styles.ShortKey = lipgloss.NewStyle().Foreground(colors.LightGray())
	m.help.Styles.ShortDesc = lipgloss.NewStyle().Foreground(colors.Gray())
	m.help.Styles.FullKey = lipgloss.NewStyle().Foreground(colors.Pink())
	m.help.Styles.FullDesc = lipgloss.NewStyle().Foreground(colors.LightGray())
	applyListTheme(&m.list)
	applyFilepickerTheme(&m.filepicker)
	m.logoCache = ""
	// Rebuild progress bar colors for all existing downloads so the gradient
	// matches the newly loaded palette rather than the one active at creation time.
	for _, d := range m.downloads {
		d.progress = progress.New(
			progress.WithSpringOptions(0.5, 0.1),
			progress.WithColors(colors.ProgressStart(), colors.ProgressEnd()),
			progress.WithScaled(true),
		)
	}
}

func applyFilepickerTheme(fp *filepicker.Model) {
	if fp == nil {
		return
	}

	fp.Styles.Cursor = lipgloss.NewStyle().Foreground(colors.Pink())
	fp.Styles.Symlink = lipgloss.NewStyle().Foreground(colors.Cyan())
	fp.Styles.Directory = lipgloss.NewStyle().Foreground(colors.Blue())
	fp.Styles.File = lipgloss.NewStyle().Foreground(colors.White())
	fp.Styles.DisabledFile = lipgloss.NewStyle().Foreground(colors.Gray())
	fp.Styles.Permission = lipgloss.NewStyle().Foreground(colors.Gray())
	fp.Styles.Selected = lipgloss.NewStyle().Foreground(colors.Pink()).Bold(true)
	fp.Styles.DisabledSelected = lipgloss.NewStyle().Foreground(colors.LightGray())
	fp.Styles.FileSize = lipgloss.NewStyle().Foreground(colors.Gray()).Width(7).Align(lipgloss.Right)
	fp.Styles.EmptyDirectory = lipgloss.NewStyle().Foreground(colors.Gray()).Padding(0, 2)
}
