package config

// Category defines a download category for auto-sorting.
type Category struct {
	Name       string   `json:"name"`
	Extensions []string `json:"extensions,omitempty"` // e.g. [".mp4", ".mkv"]
	Pattern    string   `json:"pattern,omitempty"`    // regex for filename
	SubDir     string   `json:"sub_dir"`              // subdir under default download dir
	Path       string   `json:"path,omitempty"`       // absolute path override
}

// DefaultCategories returns the default set of download categories.
func DefaultCategories() []Category {
	return []Category{
		{
			Name:       "Video",
			Extensions: []string{".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".mpg", ".mpeg", ".3gp"},
			SubDir:     "Video",
		},
		{
			Name:       "Music",
			Extensions: []string{".mp3", ".flac", ".wav", ".aac", ".ogg", ".wma", ".m4a", ".opus"},
			SubDir:     "Music",
		},
		{
			Name:       "Compressed",
			Extensions: []string{".zip", ".rar", ".7z", ".tar", ".gz", ".bz2", ".xz", ".zst", ".tgz"},
			SubDir:     "Compressed",
		},
		{
			Name:       "Documents",
			Extensions: []string{".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods", ".txt", ".rtf", ".csv", ".epub"},
			SubDir:     "Documents",
		},
		{
			Name:       "Programs",
			Extensions: []string{".exe", ".msi", ".deb", ".rpm", ".appimage", ".dmg", ".pkg", ".flatpak", ".snap", ".sh", ".run", ".bin"},
			SubDir:     "Programs",
		},
		{
			Name:       "Images",
			Extensions: []string{".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg", ".webp", ".ico", ".tiff", ".psd"},
			SubDir:     "Images",
		},
	}
}
