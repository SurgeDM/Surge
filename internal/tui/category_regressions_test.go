package tui

import (
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/download"
)

func newCategoryTestModel(t *testing.T, settings *config.Settings) RootModel {
	t.Helper()
	ch := make(chan any, 16)
	pool := download.NewWorkerPool(ch, 1)
	svc := core.NewLocalDownloadServiceWithInput(pool, ch)
	t.Cleanup(func() { _ = svc.Shutdown() })
	return RootModel{
		Settings: settings,
		Service:  svc,
		list:     NewDownloadList(80, 20),
		keys:     Keys,
		inputs:   []textinput.Model{textinput.New(), textinput.New(), textinput.New(), textinput.New()},
	}
}

func TestStartDownload_RoutesDefaultPathWithURLDerivedFilename(t *testing.T) {
	rootDir := t.TempDir()
	imageDir := filepath.Join(rootDir, "images")

	settings := config.DefaultSettings()
	settings.General.CategoryEnabled = true
	settings.General.DefaultDownloadDir = rootDir
	settings.General.Categories = []config.Category{
		{Name: "Images", Pattern: `(?i)\.(jpg|jpeg|png)$`, Path: imageDir},
	}

	m := newCategoryTestModel(t, settings)
	m, _ = m.startDownload("https://example.com/screenshot.jpg", nil, nil, rootDir, true, "", "")

	if len(m.downloads) != 1 {
		t.Fatalf("expected 1 download, got %d", len(m.downloads))
	}
	if got, want := m.downloads[0].Destination, filepath.Join(imageDir, "screenshot.jpg"); got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
}

func TestUpdate_InputSubmit_BlankPathUsesDefaultPathRouting(t *testing.T) {
	rootDir := t.TempDir()
	musicDir := filepath.Join(rootDir, "music")

	settings := config.DefaultSettings()
	settings.General.CategoryEnabled = true
	settings.General.WarnOnDuplicate = false
	settings.General.DefaultDownloadDir = rootDir
	settings.General.Categories = []config.Category{
		{Name: "Music", Pattern: `(?i)\.(mp3|flac)$`, Path: musicDir},
	}

	m := newCategoryTestModel(t, settings)
	m.state = InputState
	m.focusedInput = 3
	m.inputs[0].SetValue("https://example.com/song.mp3")
	m.inputs[2].SetValue("")
	m.inputs[3].SetValue("song.mp3")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(RootModel)

	if len(m2.downloads) != 1 {
		t.Fatalf("expected 1 download, got %d", len(m2.downloads))
	}
	if got, want := m2.downloads[0].Destination, filepath.Join(musicDir, "song.mp3"); got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
}

func TestUpdate_DuplicateContinuePreservesDefaultPathRouting(t *testing.T) {
	rootDir := t.TempDir()
	videoDir := filepath.Join(rootDir, "videos")

	settings := config.DefaultSettings()
	settings.General.CategoryEnabled = true
	settings.General.DefaultDownloadDir = rootDir
	settings.General.Categories = []config.Category{
		{Name: "Videos", Pattern: `(?i)\.mp4$`, Path: videoDir},
	}

	m := newCategoryTestModel(t, settings)
	m.state = DuplicateWarningState
	m.pendingURL = "https://example.com/movie.mp4"
	m.pendingPath = rootDir
	m.pendingIsDefaultPath = true
	m.pendingFilename = "movie.mp4"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m2 := updated.(RootModel)

	if len(m2.downloads) != 1 {
		t.Fatalf("expected 1 download, got %d", len(m2.downloads))
	}
	if got, want := m2.downloads[0].Destination, filepath.Join(videoDir, "movie.mp4"); got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
}

func TestUpdate_ExtensionConfirmBlankPathUsesDefaultPathRouting(t *testing.T) {
	rootDir := t.TempDir()
	docDir := filepath.Join(rootDir, "docs")

	settings := config.DefaultSettings()
	settings.General.CategoryEnabled = true
	settings.General.WarnOnDuplicate = false
	settings.General.DefaultDownloadDir = rootDir
	settings.General.Categories = []config.Category{
		{Name: "Documents", Pattern: `(?i)\.pdf$`, Path: docDir},
	}

	m := newCategoryTestModel(t, settings)
	m.state = ExtensionConfirmationState
	m.pendingURL = "https://example.com/report.pdf"
	m.inputs[2].SetValue("")
	m.inputs[3].SetValue("report.pdf")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(RootModel)

	if len(m2.downloads) != 1 {
		t.Fatalf("expected 1 download, got %d", len(m2.downloads))
	}
	if got, want := m2.downloads[0].Destination, filepath.Join(docDir, "report.pdf"); got != want {
		t.Fatalf("destination = %q, want %q", got, want)
	}
}

func TestUpdate_CategoryManagerEscRemovesNewPlaceholder(t *testing.T) {
	settings := config.DefaultSettings()
	settings.General.Categories = []config.Category{
		{Name: "Existing", Pattern: `(?i)\.txt$`, Path: "docs"},
		{Name: "New Category"},
	}

	m := RootModel{
		state:         CategoryManagerState,
		Settings:      settings,
		keys:          Keys,
		catMgrCursor:  1,
		catMgrEditing: true,
		catMgrIsNew:   true,
	}
	for i := range m.catMgrInputs {
		m.catMgrInputs[i] = textinput.New()
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(RootModel)

	if m2.catMgrEditing {
		t.Fatal("expected category manager to leave edit mode")
	}
	if m2.catMgrIsNew {
		t.Fatal("expected catMgrIsNew to be cleared")
	}
	if got, want := len(m2.Settings.General.Categories), 1; got != want {
		t.Fatalf("category count = %d, want %d", got, want)
	}
}

func TestGetFilteredDownloads_AppliesCategoryFilter(t *testing.T) {
	settings := config.DefaultSettings()
	settings.General.CategoryEnabled = true
	settings.General.Categories = []config.Category{
		{Name: "Videos", Pattern: `(?i)\.mp4$`},
		{Name: "Documents", Pattern: `(?i)\.pdf$`},
	}

	m := RootModel{
		Settings:       settings,
		activeTab:      TabQueued,
		categoryFilter: "Videos",
		downloads: []*DownloadModel{
			NewDownloadModel("d1", "https://example.com/movie.mp4", "movie.mp4", 0),
			NewDownloadModel("d2", "https://example.com/report.pdf", "report.pdf", 0),
			NewDownloadModel("d3", "https://example.com/blob.bin", "blob.bin", 0),
		},
	}

	filtered := m.getFilteredDownloads()
	if len(filtered) != 1 || filtered[0].ID != "d1" {
		t.Fatalf("videos filter returned %+v", filtered)
	}

	m.categoryFilter = "Uncategorized"
	filtered = m.getFilteredDownloads()
	if len(filtered) != 1 || filtered[0].ID != "d3" {
		t.Fatalf("uncategorized filter returned %+v", filtered)
	}
}
