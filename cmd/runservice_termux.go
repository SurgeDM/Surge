//go:build android

package cmd

// RunService on Android (Termux) always runs interactively since there is no
// kardianos/service-compatible service manager. Users can still manage Surge
// as a runit service via 'surge service install/start/stop' which uses sv directly.
func RunService() error {
	return rootCmd.Execute()
}
