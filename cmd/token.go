package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print the auth token used by the Surge daemon",
	Long: `Print the auth token for the currently running Surge server.

When a local server is detected, the token it is using is printed.
When the system service is running (started with 'surge service start'), use
'surge service token' to read its dedicated token.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isElevated() && checkSystemServiceRunning() {
			return fmt.Errorf("system service is running but its token could not be read. Try running 'sudo surge token' or 'surge service token' with elevated privileges")
		}

		// Read the persisted token directly — intentionally bypasses --token /
		// SURGE_TOKEN so that `surge token` always reports what the local daemon
		// is actually using, not an override that could mislead scripts.
		if details, ok := getActiveConnectionDetails(); ok && details.token != "" {
			fmt.Println(details.token)
			return nil
		}

		if isElevated() {
			return fmt.Errorf("no active local server found. To get the system service token, use 'surge service token'")
		}

		token := ensureAuthToken()
		fmt.Println(token)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tokenCmd)
}
