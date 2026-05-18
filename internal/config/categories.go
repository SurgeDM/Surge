package config

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/SurgeDM/Surge/internal/utils"
)

// Category defines a download category for auto-sorting.
type Category struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Pattern     string `json:"pattern"`
	Path        string `json:"path"`
}

func (c *Category) Validate() error {
	if c == nil {
		return errors.New("category cannot be nil")
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("category name cannot be empty")
	}
	if strings.TrimSpace(c.Pattern) == "" {
		return errors.New("category pattern cannot be empty")
	}
	if _, err := regexp.Compile(strings.TrimSpace(c.Pattern)); err != nil {
		return err
	}
	if strings.TrimSpace(c.Path) == "" {
		return errors.New("category path cannot be empty")
	}
	return nil
}

func existingDirOrFallback(dir, fallback string) string {
	trimmed := strings.TrimSpace(dir)
	if trimmed != "" {
		if info, err := os.Stat(trimmed); err == nil && info.IsDir() {
			return trimmed
		}
	}
	return fallback
}

// DefaultCategories returns the default set of download categories.
func DefaultCategories() []Category {
	downloadsDir := strings.TrimSpace(GetDownloadsDir())
	if downloadsDir == "" {
		downloadsDir = "."
	}

	videosDir := existingDirOrFallback(GetVideosDir(), downloadsDir)

	musicDir := existingDirOrFallback(GetMusicDir(), downloadsDir)

	documentsDir := existingDirOrFallback(GetDocumentsDir(), downloadsDir)

	picturesDir := existingDirOrFallback(GetPicturesDir(), downloadsDir)

	return []Category{
		{
			Name:        "Videos",
			Description: "MP4s, MKVs, AVIs, and other video files.",
			Pattern:     `(?i)\.(mp4|mkv|avi|mov|wmv|flv|webm|m4v|mpg|mpeg|3gp)$`,
			Path:        videosDir,
		},
		{
			Name:        "Music",
			Description: "MP3s, FLACs, and other audio files.",
			Pattern:     `(?i)\.(mp3|flac|wav|aac|ogg|wma|m4a|opus)$`,
			Path:        musicDir,
		},
		{
			Name:        "Compressed",
			Description: "ZIPs, RARs, and other archive files.",
			Pattern:     `(?i)\.(zip|rar|7z|tar|gz|bz2|xz|zst|tgz)$`,
			Path:        downloadsDir, // Default to downloads, can be customized
		},
		{
			Name:        "Documents",
			Description: "PDFs, Word docs, spreadsheets, etc.",
			Pattern:     `(?i)\.(pdf|doc|docx|xls|xlsx|ppt|pptx|odt|ods|txt|rtf|csv|epub)$`,
			Path:        documentsDir,
		},
		{
			Name:        "Programs",
			Description: "Executables, installers, and scripts.",
			Pattern:     `(?i)\.(exe|msi|deb|rpm|appimage|dmg|pkg|flatpak|snap|sh|run|bin)$`,
			Path:        downloadsDir,
		},
		{
			Name:        "Images",
			Description: "JPEGs, PNGs, and other image files.",
			Pattern:     `(?i)\.(jpg|jpeg|png|gif|bmp|svg|webp|ico|tiff|psd)$`,
			Path:        picturesDir,
		},
	}
}

var (
	patternCache = make(map[string]*regexp.Regexp)
	patternMu    sync.RWMutex
)

// getCompiledPattern returns a compiled regular expression, using a cache to avoid recompiling.
func getCompiledPattern(pattern string) *regexp.Regexp {
	patternMu.RLock()
	re, ok := patternCache[pattern]
	patternMu.RUnlock()
	if ok {
		return re
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		patternMu.Lock()
		patternCache[pattern] = nil
		patternMu.Unlock()
		return nil
	}

	patternMu.Lock()
	patternCache[pattern] = re
	patternMu.Unlock()
	return re
}

// GetCategoryForFile returns the last matching category so user-added rules can
// override broader defaults that appear earlier in the list.
func GetCategoryForFile(filename string, categories []Category) (*Category, error) {
	if filename == "" || len(categories) == 0 {
		return nil, nil
	}

	var matched *Category

	for i := range categories {
		cat := &categories[i]
		if cat.Pattern == "" {
			continue
		}

		re := getCompiledPattern(cat.Pattern)
		if re != nil && re.MatchString(filename) {
			if matched != nil {
				utils.Debug("Config: Category pattern %q matched %q, overriding earlier match %q", cat.Pattern, filename, matched.Pattern)
			}
			matched = cat
		}
	}

	return matched, nil
}

// ResolveCategoryPath returns the Path of a category.
func ResolveCategoryPath(cat *Category, defaultDownloadDir string) string {
	defaultPath := strings.TrimSpace(defaultDownloadDir)
	if cat == nil {
		return defaultPath
	}
	trimmed := strings.TrimSpace(cat.Path)
	if trimmed == "" {
		return defaultPath
	}
	return trimmed
}

// CategoryNames returns a slice of category names.
func CategoryNames(categories []Category) []string {
	names := make([]string, len(categories))
	for i, cat := range categories {
		names[i] = cat.Name
	}
	return names
}
