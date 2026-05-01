//go:build !android

package cmd

import (
	"github.com/kardianos/service"
)

// RunService handles the application execution, checking if it should run as a service.
// On non-Android platforms, we use kardianos/service's detection to decide whether
// to run interactively or as a managed service.
func RunService() error {
	s, err := GetService()
	if err != nil {
		return rootCmd.Execute()
	}

	if service.Interactive() {
		return rootCmd.Execute()
	}

	return s.Run()
}
