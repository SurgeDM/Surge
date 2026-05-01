//go:build android

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunServiceDoesNotHangOnAndroid(t *testing.T) {
	// On Android, RunService always calls rootCmd.Execute() directly.
	// Use --help which exits immediately to confirm no hang.
	rootCmd.SetArgs([]string{"--help"})
	defer rootCmd.SetArgs(nil)

	err := RunService()
	assert.NoError(t, err)
}
