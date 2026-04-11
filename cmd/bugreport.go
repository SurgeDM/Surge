package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/SurgeDM/Surge/internal/bugreport"
	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/spf13/cobra"
)

type bugReportTarget int

const (
	bugReportCore bugReportTarget = iota
	bugReportExtension
)

var openBrowser = utils.OpenBrowser

var bugReportCmd = &cobra.Command{
	Use:   "bug-report",
	Short: "Open a pre-filled GitHub bug report",
	Long:  `Open an interactive GitHub bug report flow for core or extension issues.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBugReportCommand(cmd)
	},
}

func runBugReportCommand(cmd *cobra.Command) error {
	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	target, err := promptBugReportTarget(reader, out)
	if err != nil {
		return err
	}

	var reportURL string
	switch target {
	case bugReportCore:
		includeSystemDetails, err := promptYesNo(reader, out, "Include system details in issue body? [Y/n]: ", true)
		if err != nil {
			return err
		}

		includeLatestLogPath, err := promptYesNo(reader, out, "Include latest debug log path in issue body? [Y/n]: ", true)
		if err != nil {
			return err
		}
		reportURL = bugreport.CoreBugReportURL(bugreport.CoreReportOptions{
			Version:              Version,
			Commit:               Commit,
			IncludeSystemDetails: includeSystemDetails,
			IncludeLatestLogPath: includeLatestLogPath,
		})
	case bugReportExtension:
		reportURL = bugreport.ExtensionBugReportURL()
	default:
		return fmt.Errorf("unsupported bug report target")
	}

	if reportURL == "" {
		return fmt.Errorf("failed to build bug report URL")
	}

	openNow, err := promptYesNo(reader, out, "Open browser now? [Y/n]: ", true)
	if err != nil {
		return err
	}

	if !openNow {
		printManualURL(out, "Browser launch skipped. Please open this URL manually:", reportURL)
		return nil
	}

	fmt.Fprintln(out, "Opening browser to file bug report...")
	if err := openBrowser(reportURL); err != nil {
		printManualURL(out, "Could not open browser. Please open this URL manually:", reportURL)
		return nil
	}

	return nil
}

func printManualURL(out io.Writer, message, reportURL string) {
	fmt.Fprintf(out, "%s\n\n%s\n", message, reportURL)
}

func promptBugReportTarget(reader *bufio.Reader, out io.Writer) (bugReportTarget, error) {
	for {
		fmt.Fprintln(out, "What would you like to report?")
		fmt.Fprintln(out, "  1) Surge Core (CLI/TUI/server)")
		fmt.Fprintln(out, "  2) Browser Extension")
		fmt.Fprint(out, "Choose [1/2] (default 1): ")

		choice, eof, err := readPromptLine(reader)
		if err != nil {
			return 0, fmt.Errorf("failed to read bug report target: %w", err)
		}

		switch strings.ToLower(choice) {
		case "", "1", "core", "c":
			return bugReportCore, nil
		case "2", "extension", "ext", "e":
			return bugReportExtension, nil
		default:
			fmt.Fprintln(out, "Invalid selection. Enter 1 for Core or 2 for Extension.")
			if eof {
				return 0, fmt.Errorf("invalid bug report target: %q", choice)
			}
		}
	}
}

func promptYesNo(reader *bufio.Reader, out io.Writer, prompt string, defaultYes bool) (bool, error) {
	for {
		fmt.Fprint(out, prompt)

		choice, eof, err := readPromptLine(reader)
		if err != nil {
			return false, fmt.Errorf("failed to read selection: %w", err)
		}

		switch strings.ToLower(choice) {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(out, "Invalid selection. Enter y or n.")
			if eof {
				return false, fmt.Errorf("invalid yes/no selection: %q", choice)
			}
		}
	}
}

func readPromptLine(reader *bufio.Reader) (string, bool, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			return strings.TrimSpace(line), true, nil
		}
		return "", false, err
	}
	return strings.TrimSpace(line), false, nil
}

func init() {
	rootCmd.AddCommand(bugReportCmd)
}
