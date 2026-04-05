package debrid

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnrestrict_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/unrestrict/link", r.URL.Path)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer")

		_ = json.NewEncoder(w).Encode(UnrestrictResult{
			ID:       "test123",
			Filename: "file.zip",
			FileSize: 1024,
			Download: "https://direct.example.com/file.zip",
			Host:     "mega.nz",
		})
	}))
	defer server.Close()

	client := NewClient("test-api-key")
	client.baseURL = server.URL

	result, err := client.Unrestrict("https://mega.nz/file/abc")
	require.NoError(t, err)
	assert.Equal(t, "file.zip", result.Filename)
	assert.Equal(t, "https://direct.example.com/file.zip", result.Download)
}

func TestUnrestrict_NoAPIKey(t *testing.T) {
	client := NewClient("")
	_, err := client.Unrestrict("https://mega.nz/file/abc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API key is required")
}

func TestUnrestrict_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(apiError{
			ErrorCode: 8,
			Error:     "Bad token",
		})
	}))
	defer server.Close()

	client := NewClient("bad-key")
	client.baseURL = server.URL

	_, err := client.Unrestrict("https://mega.nz/file/abc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Bad token")
}

func TestSupportedHosts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/hosts/domains", r.URL.Path)
		_ = json.NewEncoder(w).Encode([]string{"mega.nz", "rapidgator.net", "uploaded.net"})
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL

	hosts, err := client.SupportedHosts()
	require.NoError(t, err)
	assert.Contains(t, hosts, "mega.nz")
}

func TestIsSupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]string{"mega.nz", "rapidgator.net"})
	}))
	defer server.Close()

	client := NewClient("test-key")
	client.baseURL = server.URL

	assert.True(t, client.IsSupported("https://mega.nz/file/abc"))
	assert.True(t, client.IsSupported("https://www.mega.nz/file/abc"))
	assert.False(t, client.IsSupported("https://example.com/file"))
}
