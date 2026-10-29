package cmd

import (
	"fmt"
	"strings"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/spf13/cobra"
)

var addCmd = newAddCommand()

func newAddCommand() *cobra.Command {
	command := &cobra.Command{
		Use: "add [url]...", Aliases: []string{"get"},
		Short: "Add a new download to the running Surge instance",
		Long:  "Add URLs to a running Surge instance and report acceptance receipts, not completed downloads.",
		RunE:  runAdd,
	}
	command.Flags().StringP("batch", "b", "", "File containing URLs to download (one per line)")
	command.Flags().StringP("output", "o", "", "Output directory (defaults to current working directory)")
	command.Flags().Bool("confirm", false, "Request confirmation in the running TUI before starting downloads")
	command.Flags().String("output-version", "", "Use a versioned JSON contract (supported: 1)")
	configureManifest(command)
	command.Flags().Bool("json", false, "Write version 1 acceptance receipts")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clioutput.NewError(clioutput.InvalidArguments, "Invalid command flags.", err)
	})
	return command
}

func runAdd(cmd *cobra.Command, args []string) error {
	version, _ := cmd.Flags().GetString("output-version")
	if cmd.Flags().Changed("output-version") && version != "1" {
		return clioutput.NewError(clioutput.InvalidArguments, "Only --output-version 1 is supported.", nil)
	}
	prepared, hasManifest, err := manifestRequests(cmd, args)
	if err != nil {
		return err
	}
	key, _ := cmd.Flags().GetString("idempotency-key")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		version = "1"
	}
	batchFile, _ := cmd.Flags().GetString("batch")
	output, _ := cmd.Flags().GetString("output")
	confirm, _ := cmd.Flags().GetBool("confirm")
	urls := append([]string(nil), args...)
	if batchFile != "" {
		fileURLs, err := readBatchInputs(cmd, batchFile)
		if err != nil {
			return clioutput.NewError(clioutput.InvalidArguments, "Could not read the batch URL file.", fmt.Errorf("error reading batch file: %w", err))
		}
		urls = append(urls, fileURLs...)
	}
	if len(urls) == 0 && !hasManifest {
		return clioutput.NewError(clioutput.InvalidArguments, "Provide at least one URL or a nonempty --batch file.", nil)
	}
	resolvedOutput := resolveClientOutputPath(output)
	if hasManifest || key != "" {
		if confirm {
			return usageError("Idempotent and manifest submissions require immediate acceptance.")
		}
		if !hasManifest {
			for _, input := range urls {
				url, mirrors, e := parseAndNormalizeURLArg(input)
				if e != nil || url == "" {
					return usageError("Invalid download URL; no work was submitted.")
				}
				prepared = append(prepared, DownloadRequest{URL: url, Mirrors: mirrors, Path: resolvedOutput, SkipApproval: true})
			}
		}
		hasValidInput := false
		for _, request := range prepared {
			if request.inputFailure == nil {
				hasValidInput = true
				break
			}
		}
		if !hasValidInput {
			items := make([]submissionItem, 0, len(prepared))
			for index, request := range prepared {
				items = append(items, submissionItem{InputIndex: index + 1, ClientRef: request.ClientRef, Status: "not_submitted", Error: request.inputFailure})
			}
			return writeSubmissionResults(cmd, version, items)
		}
	}
	if err := initializeClientState(); err != nil {
		return err
	}
	baseURL, token, err := resolveAPIConnection(true)
	if err != nil {
		return err
	}
	var items []submissionItem
	if hasManifest || key != "" {
		items, err = submitPrepared(cmd, prepared, baseURL, token, key)
		if err != nil {
			return err
		}
	} else if batchFile != "" && confirm {
		items = collectConfirmedBatch(urls, resolvedOutput, baseURL, token)
	} else {
		items = collectSubmissions(urls, resolvedOutput, baseURL, token, confirm)
	}
	return writeSubmissionResults(cmd, version, items)
}

func writeSubmissionResults(cmd *cobra.Command, version string, items []submissionItem) error {
	err := submissionFailure(items)
	if version == "1" {
		if err != nil {
			return &commandResultError{result: clioutput.FailedItems("add", items, err), cause: err}
		}
		return clioutput.WriteJSON(cmd.OutOrStdout(), clioutput.Success("add", items))
	}
	var text, diagnostics strings.Builder
	for _, item := range items {
		switch item.Status {
		case "queued":
			fmt.Fprintf(&text, "Input %d: queued %q\n", item.InputIndex, item.ID)
		case "pending_approval":
			fmt.Fprintf(&text, "Input %d: awaiting approval (request %q)\n", item.InputIndex, item.RequestID)
		default:
			fmt.Fprintf(&text, "Input %d: %s\n", item.InputIndex, item.Status)
		}
		if item.Error != nil {
			fmt.Fprintf(&diagnostics, "Input %d: %s\n", item.InputIndex, item.Error.Message)
		}
	}
	if writeErr := clioutput.WriteText(cmd.OutOrStdout(), text.String()); writeErr != nil {
		return writeErr
	}
	if diagnostics.Len() > 0 {
		if writeErr := clioutput.WriteText(cmd.ErrOrStderr(), diagnostics.String()); writeErr != nil {
			return writeErr
		}
	}
	return err
}

func init() { rootCmd.AddCommand(addCmd) }
