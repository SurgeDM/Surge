package config

import "errors"

// Category defines a download category for auto-sorting.
type Category struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Pattern     string `json:"pattern"`
	Path        string `json:"path"`
}

func (c *Category) Validate() error {
	if c.Name == "" {
		return errors.New("category name cannot be empty")
	}
	if c.Pattern == "" {
		return errors.New("category pattern cannot be empty")
	}
	if c.Path == "" {
		return errors.New("category path cannot be empty")
	}
	return nil
}

// DefaultCategories returns the default set of download categories.
func DefaultCategories() []Category {
	return []Category{
		{
			Name:        "Videos",
			Description: "MP4s, MKVs, AVIs, and other video files.",
			Pattern:     `(?i)\.(mp4|mkv|avi|mov|wmv|flv|webm|m4v|mpg|mpeg|3gp)$`,
			Path:        GetVideosDir(),
		},
		{
			Name:        "Music",
			Description: "MP3s, FLACs, and other audio files.",
			Pattern:     `(?i)\.(mp3|flac|wav|aac|ogg|wma|m4a|opus)$`,
			Path:        GetMusicDir(),
		},
		{
			Name:        "Compressed",
			Description: "ZIPs, RARs, and other archive files.",
			Pattern:     `(?i)\.(zip|rar|7z|tar|gz|bz2|xz|zst|tgz)$`,
			Path:        GetDownloadsDir(), // Default to downloads, can be customized
		},
		{
			Name:        "Documents",
			Description: "PDFs, Word docs, spreadsheets, etc.",
			Pattern:     `(?i)\.(pdf|doc|docx|xls|xlsx|ppt|pptx|odt|ods|txt|rtf|csv|epub)$`,
			Path:        GetDocumentsDir(),
		},
		{
			Name:        "Programs",
			Description: "Executables, installers, and scripts.",
			Pattern:     `(?i)\.(exe|msi|deb|rpm|appimage|dmg|pkg|flatpak|snap|sh|run|bin)$`,
			Path:        GetDownloadsDir(),
		},
		{
			Name:        "Images",
			Description: "JPEGs, PNGs, and other image files.",
			Pattern:     `(?i)\.(jpg|jpeg|png|gif|bmp|svg|webp|ico|tiff|psd)$`,
			Path:        GetPicturesDir(),
		},
	}
}
