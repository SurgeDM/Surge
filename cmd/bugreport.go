package cmd

import (
	"fmt"

	"github.com/SurgeDM/Surge/internal/bugreport"
	"github.com/spf13/cobra"
)

var bugReportCmd = &cobra.Command{
	Use:   "bug-report",
	Short: "Open a pre-filled GitHub bug report",
	Long:  `Open a GitHub bug report with version, commit, and environment details pre-filled.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		reportURL := bugreport.BugReportURL(Version, Commit)
		if reportURL == "" {
			return fmt.Errorf("failed to build bug report URL")
		}

		if err := bugreport.OpenBrowser(reportURL); err != nil {
			fmt.Printf("Could not open browser. Please open this URL manually:\n%s\n", reportURL)
			return nil
		}

		fmt.Println("Opening browser to file bug report...")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(bugReportCmd)
}
