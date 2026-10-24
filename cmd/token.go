package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Print the auth token used by the Surge daemon",
	Long:  `Print the auth token for the current user's Surge server.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Read the persisted token directly — intentionally bypasses --token /
		// SURGE_TOKEN so that `surge token` always reports what the local daemon
		// is actually using, not an override that could mislead scripts.
		if details, ok := getActiveConnectionDetails(); ok && details.token != "" {
			fmt.Println(details.token)
			return nil
		}

		token := ensureAuthToken()
		fmt.Println(token)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tokenCmd)
}
