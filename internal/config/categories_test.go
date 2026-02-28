package config

import (
	"encoding/json"
	"testing"
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
		t.Errorf("Expected error for multiple matches, got nil")
	}
	if cat != nil {
		t.Errorf("Expected nil category for multiple matches, got %v", cat)
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
	if pathNil != "" {
		t.Errorf("Expected empty string for nil category, got %s", pathNil)
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
