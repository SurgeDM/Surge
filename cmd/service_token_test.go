//go:build !android

// Tests in this file mutate package-level variables (globalToken, GetService)
// and redirect os.Stdout via captureStdout.  They must NOT use t.Parallel().
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/kardianos/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// isolateTokenEnv sets up a fully isolated XDG + system-state environment
// and returns a struct with both dirs. All env vars that affect token file
// paths are reset automatically via t.Cleanup.
type tokenEnvDirs struct {
	userState   string
	systemState string
}

func isolateTokenEnv(t *testing.T) tokenEnvDirs {
	t.Helper()
	userDir := t.TempDir()
	sysDir := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", userDir)
	t.Setenv("XDG_STATE_HOME", userDir)
	t.Setenv("XDG_RUNTIME_DIR", userDir)
	t.Setenv("XDG_DATA_HOME", userDir)
	t.Setenv("HOME", userDir)
	t.Setenv("APPDATA", userDir)
	t.Setenv("USERPROFILE", userDir)
	t.Setenv("SystemRoot", userDir)
	// The key override: redirect GetSystemStateDir() to our temp dir.
	t.Setenv("SURGE_SYSTEM_STATE_DIR", sysDir)
	t.Setenv("SURGE_SYSTEM_RUNTIME_DIR", sysDir)
	// Suppress token overrides so file-path logic is exercised.
	t.Setenv("SURGE_TOKEN", "")

	origToken := globalToken
	globalToken = ""
	
	origCheck := checkSystemServiceRunning
	checkSystemServiceRunning = func() bool { return false }

	t.Cleanup(func() { 
		globalToken = origToken 
		checkSystemServiceRunning = origCheck
	})

	return tokenEnvDirs{userState: userDir, systemState: sysDir}
}

func writeUserToken(t *testing.T, token string) {
	t.Helper()
	tokenFile := filepath.Join(config.GetStateDir(), "token")
	require.NoError(t, writeTokenToFile(tokenFile, token))
}

func writeSystemTokenTo(t *testing.T, sysDir, token string) {
	t.Helper()
	tokenFile := filepath.Join(sysDir, "token")
	require.NoError(t, writeTokenToFile(tokenFile, token))
}

// ---------------------------------------------------------------------------
// resolveTokenPath
// ---------------------------------------------------------------------------

// resolveTokenPath must return the user state dir path when NOT elevated.
func TestResolveTokenPath_UserMode(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: test process is running as root; cannot test non-elevated path")
	}
	isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	got := resolveTokenPath()
	// On non-elevated processes the token lives in the user state dir.
	assert.Equal(t, filepath.Join(config.GetStateDir(), "token"), got,
		"non-elevated resolveTokenPath should point to user state dir")
}

// When elevated, resolveTokenPath must return the system state dir path.
func TestResolveTokenPath_ElevatedMode(t *testing.T) {
	if !isElevated() {
		t.Skip("skipping: test process is not root; cannot test elevated path")
	}
	dirs := isolateTokenEnv(t)

	got := resolveTokenPath()
	assert.Equal(t, filepath.Join(dirs.systemState, "token"), got,
		"elevated resolveTokenPath should point to system state dir")
}

// ---------------------------------------------------------------------------
// ensureAuthToken / persistAuthToken  (user-mode)
// ---------------------------------------------------------------------------

func TestEnsureAuthToken_CreatesTokenFile(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: test process is running as root")
	}
	isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	token := ensureAuthToken()
	require.NotEmpty(t, token)

	// Token must be persisted so the next call returns the same value.
	token2 := ensureAuthToken()
	assert.Equal(t, token, token2, "ensureAuthToken should be idempotent")
}

func TestEnsureAuthToken_ReadsExistingToken(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: test process is running as root")
	}
	isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	const want = "preset-token-value"
	writeUserToken(t, want)

	got := ensureAuthToken()
	assert.Equal(t, want, got)
}

func TestPersistAuthToken_WritesToCorrectStateDir(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: test process is running as root")
	}
	dirs := isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	const want = "persist-test-token"
	persistAuthToken(want)

	tokenFile := filepath.Join(dirs.userState, "surge", "token") // GetStateDir() = userDir/surge
	data, err := os.ReadFile(tokenFile)
	require.NoError(t, err, "token file should exist in user state dir after persistAuthToken")
	assert.Equal(t, want, strings.TrimSpace(string(data)))
}

// ---------------------------------------------------------------------------
// ensureSystemToken / readSystemServiceToken
// ---------------------------------------------------------------------------

func TestEnsureSystemToken_CreatesAndReturnsToken(t *testing.T) {
	dirs := isolateTokenEnv(t)

	token, err := ensureSystemToken()
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Idempotent.
	token2, err := ensureSystemToken()
	require.NoError(t, err)
	assert.Equal(t, token, token2, "ensureSystemToken must return the same token on repeated calls")

	// Token actually on disk at the system state dir.
	data, err := os.ReadFile(filepath.Join(dirs.systemState, "token"))
	require.NoError(t, err)
	assert.Equal(t, token, strings.TrimSpace(string(data)))
}

func TestReadSystemServiceToken_ReadsExistingToken(t *testing.T) {
	dirs := isolateTokenEnv(t)

	const want = "known-system-token"
	writeSystemTokenTo(t, dirs.systemState, want)

	got, err := readSystemServiceToken()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestReadSystemServiceToken_ErrorWhenMissing(t *testing.T) {
	isolateTokenEnv(t)
	// System state dir exists (created by isolateTokenEnv via t.TempDir) but
	// has no token file.
	_, err := readSystemServiceToken()
	assert.Error(t, err, "readSystemServiceToken should error when no token file exists")
}

// ---------------------------------------------------------------------------
// resolveTokenForConnectTarget — system-token fallback (issue #530)
// ---------------------------------------------------------------------------

// When no user token / active port match exists but a system token file does,
// resolveTokenForConnectTarget should return the system token for localhost.
func TestResolveTokenForConnectTarget_FallsBackToSystemToken(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: elevated — system and user token paths are the same")
	}

	dirs := isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	const sysToken = "system-daemon-token-530"
	writeSystemTokenTo(t, dirs.systemState, sysToken)
	// Ensure no user token file exists.
	_ = os.Remove(filepath.Join(config.GetStateDir(), "token"))

	target, err := parseConnectTarget("127.0.0.1:1700", false)
	require.NoError(t, err)

	got, err := resolveTokenForConnectTarget(target)
	require.NoError(t, err)
	assert.Equal(t, sysToken, got,
		"resolveTokenForConnectTarget should fall back to the system token for localhost when no user token matches")
}

// User token in active connection details beats system token.
func TestResolveTokenForConnectTarget_UserTokenTakesPrecedence(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: elevated — system and user token paths are the same")
	}

	dirs := isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	const userToken = "user-level-token"
	const sysToken = "system-level-token"

	writeUserToken(t, userToken)
	writeSystemTokenTo(t, dirs.systemState, sysToken)

	saveActivePort(2001)
	t.Cleanup(removeActivePort)

	target, err := parseConnectTarget("127.0.0.1:2001", false)
	require.NoError(t, err)

	got, err := resolveTokenForConnectTarget(target)
	require.NoError(t, err)
	assert.Equal(t, userToken, got,
		"active-connection user token should take precedence over system token")
}

// Explicit --token flag always wins.
func TestResolveTokenForConnectTarget_ExplicitFlagWins(t *testing.T) {
	dirs := isolateTokenEnv(t)
	writeSystemTokenTo(t, dirs.systemState, "system-token")
	require.NoError(t, config.EnsureDirs())
	writeUserToken(t, "user-token")

	origToken := globalToken
	globalToken = "explicit-flag-token"
	t.Cleanup(func() { globalToken = origToken })

	target, err := parseConnectTarget("127.0.0.1:1700", false)
	require.NoError(t, err)

	got, err := resolveTokenForConnectTarget(target)
	require.NoError(t, err)
	assert.Equal(t, "explicit-flag-token", got, "--token flag must win over all file-based tokens")
}

// ---------------------------------------------------------------------------
// surge service token subcommand
// ---------------------------------------------------------------------------

func TestServiceTokenCmd_PrintsSystemToken(t *testing.T) {
	dirs := isolateTokenEnv(t)

	const want = "service-cmd-test-token"
	writeSystemTokenTo(t, dirs.systemState, want)

	out := captureStdout(t, func() {
		err := serviceTokenCmd.RunE(serviceTokenCmd, nil)
		require.NoError(t, err)
	})
	assert.Equal(t, want, strings.TrimSpace(out))
}

func TestServiceTokenCmd_ErrorWhenNoTokenFile(t *testing.T) {
	isolateTokenEnv(t)
	// systemState dir exists (t.TempDir) but has no token file.
	err := serviceTokenCmd.RunE(serviceTokenCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not read system service token")
}

func TestServiceTokenCmd_NonRootHintInError(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: elevated process — hint text branch not exercised")
	}
	isolateTokenEnv(t)
	err := serviceTokenCmd.RunE(serviceTokenCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sudo",
		"error message should hint at using sudo for non-root users")
}

// ---------------------------------------------------------------------------
// surge service install — token generation side-effect
// ---------------------------------------------------------------------------

type mockInstallSvc struct {
	service.Service
	installCalled bool
}

func (m *mockInstallSvc) Install() error {
	m.installCalled = true
	return nil
}

func TestServiceInstallCmd_GeneratesAndPrintsToken(t *testing.T) {
	dirs := isolateTokenEnv(t)

	mock := &mockInstallSvc{}
	origGetService := GetService
	GetService = func() (service.Service, error) { return mock, nil }
	t.Cleanup(func() { GetService = origGetService })

	out := captureStdout(t, func() {
		err := serviceInstallCmd.RunE(serviceInstallCmd, nil)
		require.NoError(t, err)
	})

	assert.True(t, mock.installCalled, "Install() should be called")
	assert.Contains(t, out, "Service installed successfully")
	assert.Contains(t, out, "Service auth token:")

	// The printed token must match what was actually persisted.
	data, err := os.ReadFile(filepath.Join(dirs.systemState, "token"))
	require.NoError(t, err)
	persistedToken := strings.TrimSpace(string(data))
	assert.Contains(t, out, persistedToken,
		"printed token must match the token written to the system state dir")
}

func TestServiceInstallCmd_IdempotentToken(t *testing.T) {
	dirs := isolateTokenEnv(t)

	// Pre-write a token — install should print this existing token, not generate a new one.
	const preExisting = "pre-existing-service-token"
	writeSystemTokenTo(t, dirs.systemState, preExisting)

	mock := &mockInstallSvc{}
	origGetService := GetService
	GetService = func() (service.Service, error) { return mock, nil }
	t.Cleanup(func() { GetService = origGetService })

	out := captureStdout(t, func() {
		err := serviceInstallCmd.RunE(serviceInstallCmd, nil)
		require.NoError(t, err)
	})

	assert.Contains(t, out, preExisting,
		"install should preserve and print the pre-existing system token")
}

// ---------------------------------------------------------------------------
// surge token command
// ---------------------------------------------------------------------------

func TestTokenCmd_PrintsActiveServerToken(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: test process is running as root")
	}

	isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	const want = "active-server-token"
	writeUserToken(t, want)

	out := captureStdout(t, func() {
		err := tokenCmd.RunE(tokenCmd, nil)
		require.NoError(t, err)
	})
	assert.Equal(t, want, strings.TrimSpace(out))
}

// surge token must ignore SURGE_TOKEN / --token overrides and report the actual
// daemon token on disk. Scripts that pipe `surge token` into a config file must
// get the real token, not whatever env var happens to be set.
func TestTokenCmd_IgnoresSURGE_TOKEN_Override(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: test process is running as root")
	}

	isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	const daemonToken = "daemon-persisted-token"
	writeUserToken(t, daemonToken)

	// Set the override — surge token must NOT echo this back.
	t.Setenv("SURGE_TOKEN", "override-should-not-appear")

	out := captureStdout(t, func() {
		err := tokenCmd.RunE(tokenCmd, nil)
		require.NoError(t, err)
	})
	assert.Equal(t, daemonToken, strings.TrimSpace(out),
		"surge token must print the daemon's persisted token, not the SURGE_TOKEN override")
}

func TestTokenCmd_ErrorsIfSystemServiceRunningButUnreadable(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: test process is running as root")
	}

	isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	origCheck := checkSystemServiceRunning
	checkSystemServiceRunning = func() bool { return true }
	t.Cleanup(func() { checkSystemServiceRunning = origCheck })

	err := tokenCmd.RunE(tokenCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "system service is running but its token could not be read")
}

// When no port file matches, surge connect must prefer the system service token
// over a stale user-level token to avoid 401s against the system daemon.
func TestResolveTokenForConnectTarget_SystemTokenBeatsStaleUserToken(t *testing.T) {
	if isElevated() {
		t.Skip("skipping: elevated — system and user token paths are the same")
	}

	dirs := isolateTokenEnv(t)
	require.NoError(t, config.EnsureDirs())

	const sysToken = "system-service-token"
	const staleUserToken = "stale-old-user-token"

	// Both tokens exist on disk; no port file matches target port.
	writeSystemTokenTo(t, dirs.systemState, sysToken)
	writeUserToken(t, staleUserToken)

	origCheck := checkSystemServiceRunning
	checkSystemServiceRunning = func() bool { return true }
	t.Cleanup(func() { checkSystemServiceRunning = origCheck })

	target, err := parseConnectTarget("127.0.0.1:1700", false)
	require.NoError(t, err)

	got, err := resolveTokenForConnectTarget(target)
	require.NoError(t, err)
	assert.Equal(t, sysToken, got,
		"system token must win over stale user token in the no-port-file fallback path")
}

// ---------------------------------------------------------------------------
// Subcommand registration
// ---------------------------------------------------------------------------

func TestServiceCmd_HasTokenSubcommand(t *testing.T) {
	found := false
	for _, cmd := range serviceCmd.Commands() {
		if cmd.Name() == "token" {
			found = true
			break
		}
	}
	assert.True(t, found, "serviceCmd should register a 'token' subcommand")
}

func TestServiceCmd_HasExpectedSubcommands(t *testing.T) {
	want := []string{"install", "uninstall", "start", "stop", "status", "token"}
	names := make(map[string]bool)
	for _, cmd := range serviceCmd.Commands() {
		if !cmd.Hidden {
			names[cmd.Name()] = true
		}
	}
	for _, sub := range want {
		assert.True(t, names[sub], "serviceCmd should have subcommand %q", sub)
	}
}
