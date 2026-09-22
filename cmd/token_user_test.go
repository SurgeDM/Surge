package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
)

func isolateUserTokenPaths(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(base, "runtime"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("APPDATA", filepath.Join(base, "appdata"))
}

func TestTokenPathAlwaysUsesCurrentUserState(t *testing.T) {
	isolateUserTokenPaths(t)
	want := filepath.Join(config.GetStateDir(), "token")
	if got := resolveTokenPath(); got != want {
		t.Fatalf("resolveTokenPath() = %q, want %q", got, want)
	}
}

func TestEnsureAuthTokenStaysInUserState(t *testing.T) {
	isolateUserTokenPaths(t)
	token := ensureAuthToken()
	if token == "" {
		t.Fatal("ensureAuthToken returned an empty token")
	}

	stateToken := filepath.Join(config.GetStateDir(), "token")
	info, err := os.Stat(stateToken)
	if err != nil {
		t.Fatalf("stat state token: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state token permissions = %o, want 600", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(config.GetRuntimeDir(), "token")); !os.IsNotExist(err) {
		t.Fatalf("runtime token must not exist, stat error = %v", err)
	}
}

func TestActiveConnectionCandidatesOnlyUseCurrentUser(t *testing.T) {
	isolateUserTokenPaths(t)
	candidates := activeConnectionCandidates()
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1", len(candidates))
	}
	if candidates[0].runtimeDir != config.GetRuntimeDir() || candidates[0].stateDir != config.GetStateDir() {
		t.Fatalf("candidate = %#v, want current user runtime and state directories", candidates[0])
	}
}
