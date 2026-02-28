package config

import "errors"

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

func GetDefaultCategories() []Category {
	return []Category{
		{
			Name:        "Documents",
			Description: "PDFs, Word docs, spreadsheets, etc.",
			Pattern:     `(?i)\.(pdf|docx?|xlsx?|pptx?)$`,
			Path:        GetDocumentsDir(),
		},
		{
			Name:        "Music",
			Description: "MP3s, FLACs, and other audio files.",
			Pattern:     `(?i)\.(mp3|flac|wav|aac|ogg)$`,
			Path:        GetMusicDir(),
		},
		{
			Name:        "Videos",
			Description: "MP4s, AVIs, and other video files.",
			Pattern:     `(?i)\.(mp4|mkv|avi|mov|wmv)$`,
			Path:        GetVideosDir(),
		},
	}
}
