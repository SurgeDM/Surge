package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTryAcquireInstanceLock(t *testing.T) {
	runtimeDir := t.TempDir()

	first, acquired, err := TryAcquireInstanceLock(runtimeDir)
	require.NoError(t, err)
	require.True(t, acquired, "first acquisition should succeed")

	second, acquired, err := TryAcquireInstanceLock(runtimeDir)
	require.NoError(t, err)
	assert.False(t, acquired, "a second handle must not acquire the held lock")
	assert.Nil(t, second)

	require.NoError(t, first.Release())

	third, acquired, err := TryAcquireInstanceLock(runtimeDir)
	require.NoError(t, err)
	require.True(t, acquired, "lock should be acquirable after release")
	require.NotNil(t, third)
	t.Cleanup(func() { require.NoError(t, third.Release()) })

	lockPath := filepath.Join(runtimeDir, "surge.lock")
	_, err = os.Stat(lockPath)
	assert.NoError(t, err, "Lock file should exist")
}

func TestTryAcquireInstanceLock_BlocksAnotherProcess(t *testing.T) {
	runtimeDir := t.TempDir()
	lock, acquired, err := TryAcquireInstanceLock(runtimeDir)
	require.NoError(t, err)
	require.True(t, acquired)
	t.Cleanup(func() { require.NoError(t, lock.Release()) })

	cmd := exec.Command(os.Args[0], "-test.run=^TestInstanceLockHelperProcess$")
	cmd.Env = append(os.Environ(),
		"SURGE_LOCK_HELPER_PROCESS=1",
		"SURGE_LOCK_RUNTIME_DIR="+runtimeDir,
	)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "lock helper failed: %s", output)
}

func TestInstanceLockHelperProcess(t *testing.T) {
	if os.Getenv("SURGE_LOCK_HELPER_PROCESS") != "1" {
		return
	}

	_, acquired, err := TryAcquireInstanceLock(os.Getenv("SURGE_LOCK_RUNTIME_DIR"))
	require.NoError(t, err)
	if acquired {
		t.Fatal("helper process acquired a lock held by its parent")
	}
}
