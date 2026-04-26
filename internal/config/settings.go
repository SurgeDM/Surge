package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/SurgeDM/Surge/internal/utils"
)

// Settings holds all user-configurable application settings organized by category.
type Settings struct {
	General     GeneralSettings     `json:"general" ui_label:"General"`
	Network     NetworkSettings     `json:"network" ui_label:"Network"`
	Performance PerformanceSettings `json:"performance" ui_label:"Performance"`
	Categories  CategorySettings    `json:"categories" ui_label:"Categories"`
	Extension   ExtensionSettings   `json:"extension" ui_label:"Extension"`
}

// GeneralSettings contains application behavior settings.
type GeneralSettings struct {
	DefaultDownloadDir           string `json:"default_download_dir" ui_label:"Default Download Dir" ui_desc:"Default directory for new downloads. Leave empty to use current directory."`
	WarnOnDuplicate              bool   `json:"warn_on_duplicate" ui_label:"Warn on Duplicate" ui_desc:"Show warning when adding a download that already exists."`
	DownloadCompleteNotification bool   `json:"download_complete_notification" ui_label:"Download Complete Notification" ui_desc:"Show system notification when a download finishes."`
	AllowRemoteOpenActions       bool   `json:"allow_remote_open_actions" ui_label:"Allow Remote Open Actions" ui_desc:"Allow /open-file and /open-folder API calls from non-loopback clients. Disabled by default for security." ui_restart:"true"`
	AutoResume                   bool   `json:"auto_resume" ui_label:"Auto Resume" ui_desc:"Automatically resume paused downloads on startup." ui_restart:"true"`
	SkipUpdateCheck              bool   `json:"skip_update_check" ui_label:"Skip Update Check" ui_desc:"Disable automatic check for new versions on startup." ui_restart:"true"`

	ClipboardMonitor  bool   `json:"clipboard_monitor" ui_label:"Clipboard Monitor" ui_desc:"Watch clipboard for URLs and prompt to download them." ui_restart:"true"`
	Theme             int    `json:"theme" ui_label:"App Theme" ui_desc:"UI Theme (System, Light, Dark)."`
	ThemePath         string `json:"theme_path" ui_label:"Theme File" ui_desc:"Path to a custom .toml color scheme."`
	LogRetentionCount int    `json:"log_retention_count" ui_label:"Log Retention Count" ui_desc:"Number of recent log files to keep." ui_restart:"true"`
	LiveSpeedGraph    bool   `json:"live_speed_graph" ui_label:"Live Speed Graph" ui_desc:"Use live speed for graph instead of EMA smoothed speed."`
}

const (
	ThemeAdaptive = 0
	ThemeLight    = 1
	ThemeDark     = 2
)

// CategorySettings holds options specifically for categorizing files.
type CategorySettings struct {
	CategoryEnabled bool       `json:"category_enabled" ui_label:"Manage Categories" ui_desc:"Sort downloads into subfolders by file type. Press Enter to open Category Manager."`
	Categories      []Category `json:"categories" ui_ignored:"true"`
}

// ExtensionSettings contains settings for the browser extension.
type ExtensionSettings struct {
	ExtensionPrompt     bool   `json:"extension_prompt" ui_label:"Extension Prompt" ui_desc:"Prompt for confirmation when adding downloads via browser extension."`
	ChromeExtensionURL  string `json:"chrome_extension_url" ui_label:"Get Chrome Extension" ui_type:"link" ui_desc:"Open the Surge Chrome extension page."`
	FirefoxExtensionURL string `json:"firefox_extension_url" ui_label:"Get Firefox Extension" ui_type:"link" ui_desc:"Open the Surge Firefox extension page."`
	AuthToken           string `json:"-" ui_label:"Auth Token" ui_type:"auth_token" ui_desc:"Your authentication token. Use this to connect the Browser Extension to Surge."`
	InstructionsURL     string `json:"instructions_url" ui_label:"Setup Instructions" ui_type:"link" ui_desc:"View detailed instructions on how to set up the Surge browser extension."`
}

// NetworkSettings contains network connection parameters.
type NetworkSettings struct {
	MaxConnectionsPerHost  int    `json:"max_connections_per_host" ui_label:"Max Connections/Host" ui_desc:"Maximum concurrent connections per host (1-64)."`
	MaxConcurrentDownloads int    `json:"max_concurrent_downloads" ui_label:"Max Concurrent Downloads" ui_desc:"Maximum number of downloads running at once (1-10)." ui_restart:"true"`
	MaxConcurrentProbes    int    `json:"max_concurrent_probes" ui_label:"Max Concurrent Probes" ui_desc:"Maximum number of simultaneous server probes when adding many downloads at once (1-10)." ui_restart:"true"`
	UserAgent              string `json:"user_agent" ui_label:"User Agent" ui_desc:"Custom User-Agent string for HTTP requests. Leave empty for default."`
	ProxyURL               string `json:"proxy_url" ui_label:"Proxy URL" ui_desc:"HTTP/HTTPS proxy URL (e.g. http://127.0.0.1:1700). Leave empty to use system default."`
	CustomDNS              string `json:"custom_dns" ui_label:"Custom DNS Server" ui_desc:"Set custom DNS (e.g., 1.1.1.1:53, 94.140.14.14:53). Leave empty for system."`
	SequentialDownload     bool   `json:"sequential_download" ui_label:"Sequential Download" ui_desc:"Download pieces in order (Streaming Mode). May be slower."`
	MinChunkSize           int64  `json:"min_chunk_size" ui_label:"Min Chunk Size" ui_desc:"Minimum download chunk size in MB (e.g., 2)."`
	WorkerBufferSize       int    `json:"worker_buffer_size" ui_label:"Worker Buffer Size" ui_desc:"I/O buffer size per worker in KB (e.g., 512)."`
	DialHedgeCount         int    `json:"dial_hedge_count" ui_label:"Dial Hedge Count" ui_desc:"Number of extra connections to dial pre-emptively to avoid slow connects (0-16)."`
}

// PerformanceSettings contains performance tuning parameters.
type PerformanceSettings struct {
	MaxTaskRetries        int           `json:"max_task_retries" ui_label:"Max Task Retries" ui_desc:"Number of times to retry a failed chunk before giving up."`
	SlowWorkerThreshold   float64       `json:"slow_worker_threshold" ui_label:"Slow Worker Threshold" ui_desc:"Restart workers slower than this fraction of mean speed (0.0-1.0)."`
	SlowWorkerGracePeriod time.Duration `json:"slow_worker_grace_period" ui_label:"Slow Worker Grace" ui_desc:"Grace period before checking worker speed (e.g., 5s)."`
	StallTimeout          time.Duration `json:"stall_timeout" ui_label:"Stall Timeout" ui_desc:"Restart workers with no data for this duration (e.g., 5s)."`
	SpeedEmaAlpha         float64       `json:"speed_ema_alpha" ui_label:"Speed EMA Alpha" ui_desc:"Exponential moving average smoothing factor (0.0-1.0)."`
}

// SettingMeta provides metadata for a single setting (for UI rendering).
type SettingMeta struct {
	Key             string // JSON key name
	Label           string // Human-readable label
	Description     string // Help text displayed in right pane
	Type            string // "string", "int", "int64", "bool", "duration", "float64", "auth_token", "link"
	RequiresRestart bool   // Whether changing this setting requires an application restart
}

// GetSettingsMetadata returns metadata for all settings organized by category.
func GetSettingsMetadata() map[string][]SettingMeta {
	meta := make(map[string][]SettingMeta)
	t := reflect.TypeOf(Settings{})

	for i := 0; i < t.NumField(); i++ {
		catField := t.Field(i)
		catLabel := catField.Tag.Get("ui_label")
		if catLabel == "" {
			catLabel = catField.Name
		}

		var catMetas []SettingMeta
		catType := catField.Type
		if catType.Kind() == reflect.Struct {
			for j := 0; j < catType.NumField(); j++ {
				settingField := catType.Field(j)
				if settingField.Tag.Get("ui_ignored") == "true" {
					continue
				}

				key := settingField.Tag.Get("json")
				if key == "" {
					key = settingField.Name
				}

				label := settingField.Tag.Get("ui_label")
				if label == "" {
					label = settingField.Name
				}

				desc := settingField.Tag.Get("ui_desc")

				// Determine implicit Type
				typStr := settingField.Tag.Get("ui_type")
				if typStr == "" {
					typStr = "string"
					switch settingField.Type.Kind() {
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
						typStr = "int"
					case reflect.Int64:
						if settingField.Type.String() == "time.Duration" {
							typStr = "duration"
						} else {
							typStr = "int64"
						}
					case reflect.Bool:
						typStr = "bool"
					case reflect.Float32, reflect.Float64:
						typStr = "float64"
					}
				}

				catMetas = append(catMetas, SettingMeta{
					Key:             key,
					Label:           label,
					Description:     desc,
					Type:            typStr,
					RequiresRestart: settingField.Tag.Get("ui_restart") == "true",
				})
			}
		}
		// Only output categories that have editable GUI parameters
		if len(catMetas) > 0 {
			meta[catLabel] = catMetas
		}
	}
	return meta
}

// CategoryOrder returns the order of categories for UI tabs.
func CategoryOrder() []string {
	var order []string
	t := reflect.TypeOf(Settings{})
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		label := field.Tag.Get("ui_label")
		if label == "" {
			label = field.Name
		}

		// Ensure category has UI elements before creating a tab!
		catType := field.Type
		hasUIElements := false
		if catType.Kind() == reflect.Struct {
			for j := 0; j < catType.NumField(); j++ {
				if catType.Field(j).Tag.Get("ui_ignored") != "true" {
					hasUIElements = true
					break
				}
			}
		}

		// Only tabulate categories with active inputs
		if hasUIElements {
			order = append(order, label)
		}
	}
	return order
}

const (
	KB = 1 << 10
	MB = 1 << 20
)

// DefaultSettings returns a new Settings instance with sensible defaults.
func DefaultSettings() *Settings {

	defaultDir := GetDownloadsDir()

	return &Settings{
		General: GeneralSettings{
			DefaultDownloadDir:           defaultDir,
			WarnOnDuplicate:              true,
			DownloadCompleteNotification: true,
			AllowRemoteOpenActions:       false,
			AutoResume:                   false,

			ClipboardMonitor:  true,
			Theme:             ThemeAdaptive,
			ThemePath:         "",
			LogRetentionCount: 5,
			LiveSpeedGraph:    false,
		},
		Network: NetworkSettings{
			MaxConnectionsPerHost:  32,
			MaxConcurrentDownloads: 3,
			MaxConcurrentProbes:    3,
			UserAgent:              "", // Empty means use default UA
			ProxyURL:               "",
			CustomDNS:              "",
			SequentialDownload:     false,
			MinChunkSize:           2 * MB,
			WorkerBufferSize:       512 * KB,
			DialHedgeCount:         4,
		},
		Performance: PerformanceSettings{
			MaxTaskRetries:        3,
			SlowWorkerThreshold:   0.3,
			SlowWorkerGracePeriod: 5 * time.Second,
			StallTimeout:          3 * time.Second,
			SpeedEmaAlpha:         0.3,
		},
		Categories: CategorySettings{
			CategoryEnabled: false,
			Categories:      DefaultCategories(),
		},
		Extension: ExtensionSettings{
			ExtensionPrompt:     true,
			ChromeExtensionURL:  "https://github.com/SurgeDM/Surge/releases/latest",
			FirefoxExtensionURL: "https://addons.mozilla.org/en-US/firefox/addon/surge/",
			AuthToken:           "", // Handled specially in TUI
			InstructionsURL:     "https://github.com/SurgeDM/Surge#browser-extension",
		},
	}
}

// GetSettingsPath returns the path to the settings JSON file.
func GetSettingsPath() string {
	return filepath.Join(GetSurgeDir(), "settings.json")
}

// LoadSettings loads settings from disk. Returns defaults if file doesn't exist
// or if the JSON is corrupt, so the application can always start.
func LoadSettings() (*Settings, error) {
	path := GetSettingsPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return nil, err
	}

	settings := DefaultSettings() // Start with defaults to fill any missing fields
	if err := json.Unmarshal(data, settings); err != nil {
		utils.Debug("Warning: corrupt settings file %s: %v \u2014 using defaults", path, err)
		return DefaultSettings(), nil
	}

	return settings, nil
}

// SaveSettings saves settings to disk atomically.
func SaveSettings(s *Settings) error {
	path := GetSettingsPath()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: write to temp file, then rename
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o644); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}

// ToRuntimeConfig converts Settings to a downloader RuntimeConfig
// This is used to pass user settings to the download engine
type RuntimeConfig struct {
	MaxConnectionsPerHost int
	MaxConcurrentProbes   int
	UserAgent             string
	ProxyURL              string
	CustomDNS             string
	SequentialDownload    bool
	MinChunkSize          int64
	WorkerBufferSize      int
	DialHedgeCount        int
	MaxTaskRetries        int
	SlowWorkerThreshold   float64
	SlowWorkerGracePeriod time.Duration
	StallTimeout          time.Duration
	SpeedEmaAlpha         float64
}

// ToRuntimeConfig creates a RuntimeConfig from user Settings
func (s *Settings) ToRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{
		MaxConnectionsPerHost: s.Network.MaxConnectionsPerHost,
		MaxConcurrentProbes:   s.Network.MaxConcurrentProbes,
		UserAgent:             s.Network.UserAgent,
		ProxyURL:              s.Network.ProxyURL,
		CustomDNS:             s.Network.CustomDNS,
		SequentialDownload:    s.Network.SequentialDownload,
		MinChunkSize:          s.Network.MinChunkSize,
		WorkerBufferSize:      s.Network.WorkerBufferSize,
		DialHedgeCount:        s.Network.DialHedgeCount,
		MaxTaskRetries:        s.Performance.MaxTaskRetries,
		SlowWorkerThreshold:   s.Performance.SlowWorkerThreshold,
		SlowWorkerGracePeriod: s.Performance.SlowWorkerGracePeriod,
		StallTimeout:          s.Performance.StallTimeout,
		SpeedEmaAlpha:         s.Performance.SpeedEmaAlpha,
	}
}
