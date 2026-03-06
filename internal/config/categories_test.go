package config

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
)

func TestDefaultCategories(t *testing.T) {
	cats := DefaultCategories()
	if len(cats) != 6 {
		t.Errorf("Expected 6 default categories, got %d", len(cats))
	}

	for _, c := range cats {
		if err := c.Validate(); err != nil {
			t.Errorf("Default category %s failed validation: %v", c.Name, err)
		}
	}
}

func TestGetCategoryForFile(t *testing.T) {
	cats := []Category{
		{Name: "Video", Pattern: `(?i)\.mp4$`, Path: "/video"},
		{Name: "Doc", Pattern: `(?i)\.pdf$`, Path: "/doc"},
	}

	tests := []struct {
		filename string
		expected string
	}{
		{"test.mp4", "Video"},
		{"test.pdf", "Doc"},
		{"test.xyz", ""},
		{"TEST.MP4", "Video"},
		{"TEST.Pdf", "Doc"},
	}

	for _, tc := range tests {
		cat, err := GetCategoryForFile(tc.filename, cats)
		if err != nil {
			t.Errorf("Unexpected error for %s: %v", tc.filename, err)
		}
		if tc.expected == "" {
			if cat != nil {
				t.Errorf("Expected nil for %s, got %s", tc.filename, cat.Name)
			}
		} else {
			if cat == nil || cat.Name != tc.expected {
				t.Errorf("Expected %s for %s, got %v", tc.expected, tc.filename, cat)
			}
		}
	}
}

func TestGetCategoryForFile_Regex(t *testing.T) {
	cats := []Category{
		{Name: "ISO", Pattern: `(?i)ubuntu.*\.iso$`, Path: "/iso"},
	}

	cat, err := GetCategoryForFile("ubuntu-24.04.iso", cats)
	if err != nil || cat == nil || cat.Name != "ISO" {
		t.Errorf("Failed to match regex pattern, got: %v, err: %v", cat, err)
	}

	cat, err = GetCategoryForFile("debian.iso", cats)
	if err != nil || cat != nil {
		t.Errorf("Incorrectly matched debian.iso")
	}
}

func TestGetCategoryForFile_InvalidRegex(t *testing.T) {
	cats := []Category{
		{Name: "Bad", Pattern: `[`, Path: "/bad"},
		{Name: "Good", Pattern: `\.txt$`, Path: "/good"},
	}

	cat, err := GetCategoryForFile("test.txt", cats)
	if err != nil || cat == nil || cat.Name != "Good" {
		t.Errorf("Failed to skip invalid regex")
	}
}

func TestGetCategoryForFile_MultipleMatches(t *testing.T) {
	cats := []Category{
		{Name: "Cat1", Pattern: `\.txt$`, Path: "/1"},
		{Name: "Cat2", Pattern: `test\.txt$`, Path: "/2"},
	}

	cat, err := GetCategoryForFile("test.txt", cats)
	if err == nil {
		t.Fatalf("Expected error for multiple matches, got nil")
	}
	if cat != nil {
		t.Fatalf("Expected nil category for multiple matches, got %v", cat)
	}
	if err.Error() != "filename matches multiple categories" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestResolveCategoryPath(t *testing.T) {
	cat := &Category{Path: "/custom/path"}
	path := ResolveCategoryPath(cat, "/default")
	if path != "/custom/path" {
		t.Errorf("Expected /custom/path, got %s", path)
	}

	catNil := (*Category)(nil)
	pathNil := ResolveCategoryPath(catNil, "/default")
	if pathNil != "/default" {
		t.Errorf("Expected /default for nil category, got %s", pathNil)
	}

	catEmpty := &Category{Path: ""}
	pathEmpty := ResolveCategoryPath(catEmpty, "/default")
	if pathEmpty != "/default" {
		t.Errorf("Expected /default for empty category path, got %s", pathEmpty)
	}

	catWhitespace := &Category{Path: "   "}
	pathWhitespace := ResolveCategoryPath(catWhitespace, "/default")
	if pathWhitespace != "/default" {
		t.Errorf("Expected /default for whitespace category path, got %s", pathWhitespace)
	}

	pathTrimmedDefault := ResolveCategoryPath(catWhitespace, "   /default  ")
	if pathTrimmedDefault != "/default" {
		t.Errorf("Expected trimmed default path, got %s", pathTrimmedDefault)
	}
}

func TestCategoryValidate_RejectsWhitespaceFields(t *testing.T) {
	var nilCategory *Category
	if err := nilCategory.Validate(); err == nil || err.Error() != "category cannot be nil" {
		t.Fatalf("expected nil category validation error, got %v", err)
	}

	cat := Category{Name: "   ", Pattern: `(?i)\.txt$`, Path: "/tmp"}
	if err := cat.Validate(); err == nil || err.Error() != "category name cannot be empty" {
		t.Fatalf("expected name validation error, got %v", err)
	}

	cat = Category{Name: "Docs", Pattern: "   ", Path: "/tmp"}
	if err := cat.Validate(); err == nil || err.Error() != "category pattern cannot be empty" {
		t.Fatalf("expected pattern validation error, got %v", err)
	}

	cat = Category{Name: "Docs", Pattern: `(?i)\.txt$`, Path: "   "}
	if err := cat.Validate(); err == nil || err.Error() != "category path cannot be empty" {
		t.Fatalf("expected path validation error, got %v", err)
	}

	cat = Category{Name: "Docs", Pattern: "[", Path: "/tmp"}
	if err := cat.Validate(); err == nil {
		t.Fatal("expected invalid regex validation error, got nil")
	}
}

func TestDefaultCategories_FallbackWhenUserDirsMissing(t *testing.T) {
	tmp := t.TempDir()
	oldVideos := xdg.UserDirs.Videos
	oldMusic := xdg.UserDirs.Music
	oldDocuments := xdg.UserDirs.Documents
	oldPictures := xdg.UserDirs.Pictures
	xdg.UserDirs.Videos = filepath.Join(tmp, "missing-videos")
	xdg.UserDirs.Music = filepath.Join(tmp, "missing-music")
	xdg.UserDirs.Documents = filepath.Join(tmp, "missing-documents")
	xdg.UserDirs.Pictures = filepath.Join(tmp, "missing-pictures")
	t.Cleanup(func() {
		xdg.UserDirs.Videos = oldVideos
		xdg.UserDirs.Music = oldMusic
		xdg.UserDirs.Documents = oldDocuments
		xdg.UserDirs.Pictures = oldPictures
	})

	cats := DefaultCategories()
	pathByName := make(map[string]string, len(cats))
	for _, cat := range cats {
		pathByName[cat.Name] = cat.Path
	}

	downloadsPath := pathByName["Compressed"]
	for _, name := range []string{"Videos", "Music", "Documents", "Images"} {
		if got := pathByName[name]; got != downloadsPath {
			t.Fatalf("%s path = %q, want fallback %q", name, got, downloadsPath)
		}
	}
}

func TestCategoryJSON_RoundTrip(t *testing.T) {
	c := Category{
		Name:    "Test",
		Pattern: `\.test$`,
		Path:    "/path",
	}

	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}

	var c2 Category
	if err := json.Unmarshal(data, &c2); err != nil {
		t.Fatal(err)
	}

	if c.Pattern != c2.Pattern {
		t.Errorf("Pattern did not survive round trip: %s != %s", c.Pattern, c2.Pattern)
	}
}

func TestCategoryNames(t *testing.T) {
	// Nil input
	if names := CategoryNames(nil); len(names) != 0 {
		t.Errorf("Expected empty names for nil input, got %v", names)
	}

	// Empty input
	if names := CategoryNames([]Category{}); len(names) != 0 {
		t.Errorf("Expected empty names for empty input, got %v", names)
	}

	// Normal input
	cats := []Category{
		{Name: "A", Pattern: `\.a$`, Path: "/a"},
		{Name: "B", Pattern: `\.b$`, Path: "/b"},
	}
	names := CategoryNames(cats)
	if len(names) != 2 || names[0] != "A" || names[1] != "B" {
		t.Errorf("Expected [A B], got %v", names)
	}
}

func TestGetCategoryForFile_EmptyInputs(t *testing.T) {
	cats := []Category{
		{Name: "Doc", Pattern: `\.pdf$`, Path: "/doc"},
	}

	// Empty filename with non-empty categories
	cat, err := GetCategoryForFile("", cats)
	if err != nil || cat != nil {
		t.Errorf("Expected nil, nil for empty filename; got cat=%v, err=%v", cat, err)
	}

	// Non-empty filename with nil categories
	cat, err = GetCategoryForFile("test.pdf", nil)
	if err != nil || cat != nil {
		t.Errorf("Expected nil, nil for nil categories; got cat=%v, err=%v", cat, err)
	}

	// Non-empty filename with empty categories
	cat, err = GetCategoryForFile("test.pdf", []Category{})
	if err != nil || cat != nil {
		t.Errorf("Expected nil, nil for empty categories; got cat=%v, err=%v", cat, err)
	}
}

func TestGetCompiledPattern_Concurrent(t *testing.T) {
	// Stress-test the pattern cache with concurrent access
	patterns := []string{
		`(?i)\.mp4$`, `(?i)\.pdf$`, `(?i)\.zip$`,
		`(?i)\.jpg$`, `(?i)\.mp3$`, `[`, // invalid
	}

	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				for _, p := range patterns {
					_ = getCompiledPattern(p)
				}
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify invalid pattern returns nil
	if re := getCompiledPattern("["); re != nil {
		t.Error("Expected nil for invalid pattern")
	}

	// Verify valid pattern returns non-nil
	if re := getCompiledPattern(`(?i)\.mp4$`); re == nil {
		t.Error("Expected non-nil for valid pattern")
	}
}
