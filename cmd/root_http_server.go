package cmd

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/core"
	"github.com/surge-downloader/surge/internal/utils"
)

const serverBindHost = "0.0.0.0"

// findAvailablePort tries ports starting from 'start' until one is available
func findAvailablePort(start int) (int, net.Listener) {
	bindHost := serverBindHost
	for port := start; port < start+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bindHost, port))
		if err == nil {
			return port, ln
		}
	}
	return 0, nil
}

func bindServerListener(portFlag int) (int, net.Listener, error) {
	bindHost := serverBindHost
	if portFlag > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bindHost, portFlag))
		if err != nil {
			return 0, nil, fmt.Errorf("could not bind to port %d: %w", portFlag, err)
		}
		return portFlag, ln, nil
	}
	port, ln := findAvailablePort(1700)
	if ln == nil {
		return 0, nil, fmt.Errorf("could not find available port")
	}
	return port, ln, nil
}

// saveActivePort writes the active port for local CLI and extension discovery.
func saveActivePort(port int) {
	if err := os.MkdirAll(config.GetRuntimeDir(), 0o755); err != nil {
		utils.Debug("Error creating runtime directory for port file: %v", err)
		return
	}

	portFile := filepath.Join(config.GetRuntimeDir(), "port")
	if err := os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0o644); err != nil {
		utils.Debug("Error writing port file: %v", err)
	}
	utils.Debug("HTTP server listening on port %d", port)
}

// removeActivePort cleans up the port file on exit
func removeActivePort() {
	portFile := filepath.Join(config.GetRuntimeDir(), "port")
	if err := os.Remove(portFile); err != nil && !os.IsNotExist(err) {
		utils.Debug("Error removing port file: %v", err)
	}
}

// startHTTPServer starts the HTTP server using an existing listener
func startHTTPServer(ln net.Listener, port int, defaultOutputDir string, service core.DownloadService, tokenOverride string) {
	authToken := strings.TrimSpace(tokenOverride)
	if authToken == "" {
		authToken = ensureAuthToken()
	} else {
		persistAuthToken(authToken)
	}

	mux := http.NewServeMux()
	registerHTTPRoutes(mux, port, defaultOutputDir, service)

	// Wrap mux with Auth and CORS (CORS outermost to ensure 401/403 include headers)
	handler := corsMiddleware(authMiddleware(authToken, mux))

	server := &http.Server{Handler: handler}
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		utils.Debug("HTTP server error: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS, PUT, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Access-Control-Allow-Private-Network")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func authMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow health check without auth
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Allow OPTIONS for CORS preflight
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}

		// Check for Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			if strings.HasPrefix(authHeader, "Bearer ") {
				providedToken := strings.TrimPrefix(authHeader, "Bearer ")
				if len(providedToken) == len(token) && subtle.ConstantTimeCompare([]byte(providedToken), []byte(token)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

func ensureAuthToken() string {
	stateTokenFile := filepath.Join(config.GetStateDir(), "token")
	if token, err := readTokenFromFile(stateTokenFile); err == nil {
		return token
	}

	token := uuid.New().String()
	if err := writeTokenToFile(stateTokenFile, token); err != nil {
		utils.Debug("Failed to write token file in state dir: %v", err)
	}
	return token
}

func persistAuthToken(token string) {
	stateTokenFile := filepath.Join(config.GetStateDir(), "token")

	if err := writeTokenToFile(stateTokenFile, token); err != nil {
		utils.Debug("Failed to write token file in state dir: %v", err)
	}
}

func readTokenFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("empty token file: %s", path)
	}
	return token, nil
}

func writeTokenToFile(path string, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token), 0o600)
}
