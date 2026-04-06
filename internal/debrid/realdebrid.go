package debrid

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a Real-Debrid API client.
type Client struct {
	apiKey      string
	httpClient  *http.Client
	baseURL     string
	cachedHosts []string
	hostsCached time.Time
}

// NewClient creates a new Real-Debrid client with the given API key.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.real-debrid.com/rest/1.0",
	}
}

// UnrestrictResult holds the response from unrestricting a link.
type UnrestrictResult struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	FileSize int64  `json:"filesize"`
	Link     string `json:"link"`     // Original link
	Download string `json:"download"` // Unrestricted direct download URL
	Host     string `json:"host"`
	MimeType string `json:"mimeType"`
}

// apiError represents an error response from Real-Debrid.
type apiError struct {
	ErrorCode int    `json:"error_code"`
	Error     string `json:"error"`
}

// Unrestrict takes a hosted file URL and returns a direct download URL.
func (c *Client) Unrestrict(link string) (*UnrestrictResult, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("debrid: API key is required")
	}

	data := url.Values{}
	data.Set("link", link)

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/unrestrict/link", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("debrid: failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("debrid: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("debrid: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr apiError
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Error != "" {
			return nil, fmt.Errorf("debrid: API error %d: %s", apiErr.ErrorCode, apiErr.Error)
		}
		return nil, fmt.Errorf("debrid: unexpected status %d", resp.StatusCode)
	}

	var result UnrestrictResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("debrid: failed to parse response: %w", err)
	}

	if result.Download == "" {
		return nil, fmt.Errorf("debrid: no download URL in response")
	}

	return &result, nil
}

// SupportedHosts returns the list of supported file hosting domains.
func (c *Client) SupportedHosts() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/hosts/domains", nil)
	if err != nil {
		return nil, fmt.Errorf("debrid: failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("debrid: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("debrid: unexpected status %d", resp.StatusCode)
	}

	var domains []string
	if err := json.NewDecoder(resp.Body).Decode(&domains); err != nil {
		return nil, fmt.Errorf("debrid: failed to parse hosts response: %w", err)
	}

	return domains, nil
}

// supportedHostsCached returns the supported hosts list, caching for 1 hour
// to avoid making a live HTTP call on every invocation.
func (c *Client) supportedHostsCached() ([]string, error) {
	if c.cachedHosts != nil && time.Since(c.hostsCached) < time.Hour {
		return c.cachedHosts, nil
	}
	hosts, err := c.SupportedHosts()
	if err != nil {
		return nil, err
	}
	c.cachedHosts = hosts
	c.hostsCached = time.Now()
	return hosts, nil
}

// IsSupported checks if a URL's host is supported by Real-Debrid.
func (c *Client) IsSupported(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())

	hosts, err := c.supportedHostsCached()
	if err != nil {
		return false
	}

	for _, h := range hosts {
		h = strings.ToLower(h)
		if h == host || strings.HasSuffix(host, "."+h) {
			return true
		}
	}
	return false
}
