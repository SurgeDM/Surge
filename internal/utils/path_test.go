package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAbsPath(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "relative dot",
			in:   ".",
			want: wd,
		},
		{
			name: "relative path",
			in:   "foo",
			want: filepath.Join(wd, "foo"),
		},
		{
			name: "empty string",
			in:   "",
			want: wd,
		},
		{
			name: "already absolute",
			in:   wd,
			want: wd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsureAbsPath(tt.in)
			if got != tt.want {
				t.Errorf("EnsureAbsPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsWindowsAbsPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: `C:\Users\me\Downloads`, want: true},
		{path: `C:/Users/me/Downloads`, want: true},
		{path: `\\server\share\file.zip`, want: true},
		{path: `/tmp/downloads`, want: false},
		{path: `downloads/subdir`, want: false},
	}

	for _, tt := range tests {
		if got := IsWindowsAbsPath(tt.path); got != tt.want {
			t.Errorf("IsWindowsAbsPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestMapWindowsPathToDefaultDir(t *testing.T) {
	tests := []struct {
		name       string
		request    string
		defaultDir string
		want       string
		wantOK     bool
	}{
		{
			name:       "download root maps to default dir",
			request:    `C:/Users/me/Downloads`,
			defaultDir: `/downloads`,
			want:       filepath.Clean(`/downloads`),
			wantOK:     true,
		},
		{
			name:       "nested subdir preserved",
			request:    `C:/Users/me/Downloads/surge-repro`,
			defaultDir: `/downloads`,
			want:       filepath.Join(filepath.Clean(`/downloads`), `surge-repro`),
			wantOK:     true,
		},
		{
			name:       "custom root basename matches case-insensitively",
			request:    `D:/Archive/Downloads/Nested/Deep`,
			defaultDir: `/DownLoads`,
			want:       filepath.Join(filepath.Clean(`/DownLoads`), `Nested`, `Deep`),
			wantOK:     true,
		},
		{
			name:       "non matching root is not mapped",
			request:    `C:/Users/me/Desktop`,
			defaultDir: `/downloads`,
			wantOK:     false,
		},
		{
			name:       "empty request path",
			request:    "",
			defaultDir: `/downloads`,
			wantOK:     false,
		},
		{
			name:       "linux path is not mapped",
			request:    `/tmp/downloads`,
			defaultDir: `/downloads`,
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MapWindowsPathToDefaultDir(tt.request, tt.defaultDir)
			if ok != tt.wantOK {
				t.Fatalf("MapWindowsPathToDefaultDir(%q, %q) ok = %v, want %v", tt.request, tt.defaultDir, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("MapWindowsPathToDefaultDir(%q, %q) = %q, want %q", tt.request, tt.defaultDir, got, tt.want)
			}
		})
	}
}
