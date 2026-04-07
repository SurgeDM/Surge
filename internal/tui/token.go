package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/SurgeDM/Surge/internal/config"
)

// Cached auth token. Read once on startup or explicitly invalidated.
var authToken string

// InitAuthToken reads the token file once and caches the result.
func InitAuthToken() {
	tokenPath := filepath.Join(config.GetStateDir(), "token")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		authToken = ""
		return
	}
	authToken = strings.TrimSpace(string(data))
}

// GetAuthToken returns the cached auth token.
func GetAuthToken() string {
	return authToken
}

// ClearAuthToken resets the cached token to empty string.
func ClearAuthToken() {
	authToken = ""
}

// FormatTokenForDisplay returns a user-friendly representation of the token.
// Long tokens are truncated with an ellipsis.
func FormatTokenForDisplay(token string) string {
	if token == "" {
		return "<not set>"
	}
	if len(token) > 20 {
		return token[:10] + "..." + token[len(token)-10:]
	}
	return token
}
