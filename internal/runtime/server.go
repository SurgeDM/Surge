package runtime

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/surge-downloader/surge/internal/config"
	"github.com/surge-downloader/surge/internal/utils"
)

func FindAvailablePort(bindHost string, start int) (int, net.Listener) {
	for port := start; port < start+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bindHost, port))
		if err == nil {
			return port, ln
		}
	}
	return 0, nil
}

func BindServerListener(bindHost string, portFlag int) (int, net.Listener, error) {
	if portFlag > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bindHost, portFlag))
		if err != nil {
			return 0, nil, fmt.Errorf("could not bind to port %d: %w", portFlag, err)
		}
		return portFlag, ln, nil
	}

	port, ln := FindAvailablePort(bindHost, 1700)
	if ln == nil {
		return 0, nil, fmt.Errorf("could not find available port")
	}
	return port, ln, nil
}

func SaveActivePort(port int) {
	portFile := filepath.Join(config.GetRuntimeDir(), "port")
	if err := os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0o644); err != nil {
		utils.Debug("Error writing port file: %v", err)
	}
	utils.Debug("HTTP server listening on port %d", port)
}

func ReadActivePort() int {
	portFile := filepath.Join(config.GetRuntimeDir(), "port")
	data, err := os.ReadFile(portFile)
	if err != nil {
		return 0
	}

	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return port
}

func RemoveActivePort() {
	portFile := filepath.Join(config.GetRuntimeDir(), "port")
	if err := os.Remove(portFile); err != nil && !os.IsNotExist(err) {
		utils.Debug("Error removing port file: %v", err)
	}
}

func StartHTTPServer(ln net.Listener, tokenOverride string, handlerFactory func(authToken string) http.Handler) {
	authToken := strings.TrimSpace(tokenOverride)
	if authToken == "" {
		authToken = EnsureAuthToken()
	} else {
		PersistAuthToken(authToken)
	}

	if handlerFactory == nil {
		handlerFactory = func(string) http.Handler {
			return http.NotFoundHandler()
		}
	}

	server := &http.Server{Handler: handlerFactory(authToken)}
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		utils.Debug("HTTP server error: %v", err)
	}
}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS, PUT, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Access-Control-Allow-Private-Network")
		w.Header().Set("Access-Control-Allow-Private-Network", "true")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func AuthMiddleware(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			providedToken := strings.TrimPrefix(authHeader, "Bearer ")
			if len(providedToken) == len(token) && subtle.ConstantTimeCompare([]byte(providedToken), []byte(token)) == 1 {
				next.ServeHTTP(w, r)
				return
			}
		}

		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

func EnsureAuthToken() string {
	stateTokenFile := filepath.Join(config.GetStateDir(), "token")
	if token, err := ReadTokenFromFile(stateTokenFile); err == nil {
		return token
	}

	token := uuid.New().String()
	if err := WriteTokenToFile(stateTokenFile, token); err != nil {
		utils.Debug("Failed to write token file in state dir: %v", err)
	}
	return token
}

func PersistAuthToken(token string) {
	stateTokenFile := filepath.Join(config.GetStateDir(), "token")
	if err := WriteTokenToFile(stateTokenFile, token); err != nil {
		utils.Debug("Failed to write token file in state dir: %v", err)
	}
}

func ReadTokenFromFile(path string) (string, error) {
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

func WriteTokenToFile(path string, token string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token), 0o600)
}
