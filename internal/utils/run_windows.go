//go:build windows

package utils

import (
	"os"
	"os/exec"
)

func run(executable string, args []string, env []string) error {
	cmd := exec.Command(executable, args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		return err
	}
	// Do NOT call os.Exit(0).
	// The Go runtime will call os.Exit(0) automatically when the main
	// goroutine exits.
	// Calling it here would terminate the parent process (Explorer) prematurely.
	return nil
}
