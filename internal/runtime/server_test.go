package runtime

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/surge-downloader/surge/internal/config"
)

func TestSaveActivePort_RoundTrip(t *testing.T) {
	setupRuntimeTestEnv(t)
	if err := os.MkdirAll(config.GetRuntimeDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll(runtime dir) error = %v", err)
	}

	SaveActivePort(1707)
	if got := ReadActivePort(); got != 1707 {
		t.Fatalf("ReadActivePort() = %d, want 1707", got)
	}

	RemoveActivePort()
	if got := ReadActivePort(); got != 0 {
		t.Fatalf("ReadActivePort() after removal = %d, want 0", got)
	}
}

func TestEnsureAuthToken_GeneratesAndReusesToken(t *testing.T) {
	setupRuntimeTestEnv(t)

	tokenPath := filepath.Join(config.GetStateDir(), "token")
	token := EnsureAuthToken()
	if strings.TrimSpace(token) == "" {
		t.Fatal("expected generated auth token")
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("ReadFile(token) error = %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != token {
		t.Fatalf("persisted token = %q, want %q", got, token)
	}

	PersistAuthToken("known-token")
	if got := EnsureAuthToken(); got != "known-token" {
		t.Fatalf("EnsureAuthToken() with existing token = %q, want %q", got, "known-token")
	}
}

func TestAuthMiddleware_AllowsHealthAndValidBearerToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := AuthMiddleware("secret-token", next)

	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthResp := httptest.NewRecorder()
	handler.ServeHTTP(healthResp, healthReq)
	if healthResp.Code != http.StatusNoContent {
		t.Fatalf("/health status = %d, want %d", healthResp.Code, http.StatusNoContent)
	}

	authorizedReq := httptest.NewRequest(http.MethodGet, "/downloads", nil)
	authorizedReq.Header.Set("Authorization", "Bearer secret-token")
	authorizedResp := httptest.NewRecorder()
	handler.ServeHTTP(authorizedResp, authorizedReq)
	if authorizedResp.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d, want %d", authorizedResp.Code, http.StatusNoContent)
	}
}

func TestAuthMiddleware_RejectsInvalidBearerToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := AuthMiddleware("secret-token", next)

	req := httptest.NewRequest(http.MethodGet, "/downloads", nil)
	req.Header.Set("Authorization", "Bearer otherx-token")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestFindAvailablePort_ReturnsZeroWhenRangeOccupied(t *testing.T) {
	start, listeners := occupyContiguousPortRange(t, "127.0.0.1", 100)
	defer func() {
		for _, ln := range listeners {
			_ = ln.Close()
		}
	}()

	port, ln := FindAvailablePort("127.0.0.1", start)
	if ln != nil {
		_ = ln.Close()
	}
	if port != 0 || ln != nil {
		t.Fatalf("FindAvailablePort() = (%d, %v), want (0, nil)", port, ln)
	}
}

func occupyContiguousPortRange(t *testing.T, host string, width int) (int, []net.Listener) {
	t.Helper()

	for start := 20000; start <= 55000-width; start += width {
		listeners := make([]net.Listener, 0, width)
		ok := true

		for port := start; port < start+width; port++ {
			ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err != nil {
				ok = false
				break
			}
			listeners = append(listeners, ln)
		}

		if ok {
			return start, listeners
		}

		for _, ln := range listeners {
			_ = ln.Close()
		}
	}

	t.Fatal("failed to reserve a contiguous port range")
	return 0, nil
}
