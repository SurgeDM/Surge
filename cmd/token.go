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
	Run: func(cmd *cobra.Command, args []string) {
		// Read the persisted token directly — intentionally bypasses --token /
		// SURGE_TOKEN so that `surge token` always reports what the local daemon
		// is actually using, not an override that could mislead scripts.
		if details, ok := getActiveConnectionDetails(); ok && details.token != "" {
			fmt.Println(details.token)
			return
		}

		token := ensureAuthToken()
		fmt.Println(token)
	},
}

func init() {
	rootCmd.AddCommand(tokenCmd)
}
