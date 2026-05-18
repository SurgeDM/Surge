package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServiceCommandRegistration(t *testing.T) {
	// Verify service command is registered with correct subcommands
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "service" {
			found = true
			subcommands := cmd.Commands()
			assert.NotEmpty(t, subcommands)

			names := []string{}
			for _, sub := range subcommands {
				names = append(names, sub.Name())
			}
			assert.Contains(t, names, "install")
			assert.Contains(t, names, "uninstall")
			assert.Contains(t, names, "start")
			assert.Contains(t, names, "stop")
			assert.Contains(t, names, "status")
			break
		}
	}
	assert.True(t, found, "service command not found in rootCmd")
}
