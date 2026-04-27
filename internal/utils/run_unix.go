//go:build !windows

package utils

import "syscall"

func run(executable string, args []string, env []string) error {
	return syscall.Exec(executable, args, env)
}
