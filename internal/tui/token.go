package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/SurgeDM/Surge/internal/config"
)

var authTokenCache string

// InitAuthToken reads the authentication token from disk and caches it in memory.
// This is called once at startup to avoid re-reading from disk on every frame layout.
func InitAuthToken() {
	stateTokenFile := filepath.Join(config.GetStateDir(), "token")
	data, err := os.ReadFile(stateTokenFile)
	if err == nil {
		authTokenCache = strings.TrimSpace(string(data))
	}
}

// GetAuthToken returns the cached authentication token.
func GetAuthToken() string {
	return authTokenCache
}
