package utils

import "os/exec"

// revealFile shows path selected in Finder. Wait for the command so a failed
// reveal reaches the caller's fallback; open returns once Finder has the request.
func revealFile(path string) error {
	return buildRevealCommand(path).Run()
}

func buildRevealCommand(path string) *exec.Cmd {
	return exec.Command("open", "-R", path)
}
