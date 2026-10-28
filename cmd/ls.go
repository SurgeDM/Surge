package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/spf13/cobra"
)

var lsCmd = newListCommand()

func newListCommand() *cobra.Command {
	command := &cobra.Command{
		Use:     "ls [id]",
		Aliases: []string{"l"},
		Short:   "List downloads",
		Long:    `List all downloads from the running server or database. Optionally show details for a specific download by ID.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
				return clioutput.NewError(clioutput.InvalidArguments, "Expected at most one download ID.", err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, _ := cmd.Flags().GetBool("json")
			watch, _ := cmd.Flags().GetBool("watch")
			version, _ := cmd.Flags().GetString("output-version")
			if cmd.Flags().Changed("output-version") && version != "1" {
				return clioutput.NewError(clioutput.InvalidArguments, "Only --output-version 1 is supported.", nil)
			}
			if version == "1" && watch {
				return clioutput.NewError(clioutput.InvalidArguments, "--watch cannot be used with --output-version; v1 produces a single snapshot.", nil)
			}
			if err := validateLSFlags(len(args), jsonOutput, watch); err != nil {
				return clioutput.NewError(clioutput.InvalidArguments, err.Error(), err)
			}
			if err := initializeClientState(); err != nil {
				return err
			}

			baseURL, token, err := resolveAPIConnection(false)
			if err != nil {
				return err
			}

			strictRemote := resolveHostTarget() != ""
			lastWarning := ""
			render := func(out io.Writer) error {
				var snapshot downloadSnapshot
				var err error
				if len(args) == 1 {
					snapshot, err = readDownloadDetails(cmd.Context(), args[0], baseURL, token, strictRemote)
				} else {
					snapshot, err = readDownloadSnapshot(cmd.Context(), baseURL, token, strictRemote)
				}
				if err != nil {
					return err
				}
				if version != "1" && (len(snapshot.downloads) > 0 || baseURL != "") && snapshot.warning != "" && snapshot.warning != lastWarning {
					if err := clioutput.WriteText(cmd.ErrOrStderr(), snapshot.warning+"\n"); err != nil {
						return err
					}
				}
				lastWarning = snapshot.warning
				if version == "1" {
					return clioutput.WriteJSON(out, snapshot.result())
				}
				if watch {
					if err := clioutput.WriteText(out, "Source: "+snapshot.source+"\n"); err != nil {
						return err
					}
				}
				if len(args) == 1 {
					return printDownloadDetail(out, snapshot.downloads[0], jsonOutput)
				}
				return renderDownloads(out, jsonOutput, snapshot.downloads)
			}
			if watch {
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				return watchDownloads(cmd.Context(), cmd.OutOrStdout(), ticker.C, supportsWatchRedraw(cmd.OutOrStdout()), render)
			}
			return render(cmd.OutOrStdout())
		},
	}
	command.Flags().Bool("json", false, "Output in JSON format (legacy list/detail shapes)")
	command.Flags().Bool("watch", false, "Watch mode: refresh every second")
	command.Flags().String("output-version", "", "Use a versioned JSON contract (supported: 1)")
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clioutput.NewError(clioutput.InvalidArguments, "Invalid command flags.", err)
	})
	return command
}

func validateLSFlags(argCount int, jsonOutput, watch bool) error {
	if watch && jsonOutput {
		return errors.New("--watch cannot be used with --json because watch output is not a single JSON document")
	}
	if watch && argCount == 1 {
		return errors.New("--watch cannot be used with a download ID")
	}
	return nil
}

// downloadInfo is a unified structure for display
type downloadInfo struct {
	ID         string  `json:"id"`
	URL        string  `json:"url,omitempty"`
	Filename   string  `json:"filename"`
	Status     string  `json:"status"`
	Progress   float64 `json:"progress"`
	TotalSize  int64   `json:"total_size"`
	Downloaded int64   `json:"downloaded"`
	Speed      float64 `json:"speed,omitempty"`
}

func listDownloads(baseURL string, token string, strictRemote bool) ([]types.DownloadStatus, error) {
	snapshot, err := readDownloadSnapshot(context.Background(), baseURL, token, strictRemote)
	return snapshot.downloads, err
}

func printDownloads(out io.Writer, jsonOutput bool, baseURL string, token string, strictRemote bool) error {
	statuses, err := listDownloads(baseURL, token, strictRemote)
	if err != nil {
		return err
	}
	return renderDownloads(out, jsonOutput, statuses)
}

func renderDownloads(out io.Writer, jsonOutput bool, statuses []types.DownloadStatus) error {
	downloads := make([]downloadInfo, 0, len(statuses))
	for _, status := range statuses {
		downloads = append(downloads, downloadInfoFromStatus(status))
	}

	if jsonOutput {
		return clioutput.WriteJSON(out, downloads)
	}
	if len(downloads) == 0 {
		return clioutput.WriteText(out, "No downloads found.\n")
	}

	// Table output
	var text strings.Builder
	w := tabwriter.NewWriter(&text, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tFILENAME\tSTATUS\tPROGRESS\tSPEED\tSIZE")
	_, _ = fmt.Fprintln(w, "--\t--------\t------\t--------\t-----\t----")

	for _, d := range downloads {
		progress := fmt.Sprintf("%.1f%%", d.Progress)
		size := utils.FormatBytes(d.TotalSize)

		// Speed display
		var speed string
		if d.Speed > 0 {
			speed = utils.FormatSpeed(d.Speed)
		} else {
			speed = "-"
		}

		// Truncate ID for display
		id := safeTerminalText(d.ID)
		if len(id) > 8 {
			id = id[:8]
		}

		// Truncate filename
		filename := truncateFilenameForDisplay(safeTerminalText(d.Filename), 25)

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", id, filename, safeTerminalText(d.Status), progress, speed, size)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return clioutput.WriteText(out, text.String())
}

func showDownloadDetails(out io.Writer, partialID string, jsonOutput bool, baseURL string, token string) error {
	status, err := getDownloadDetails(partialID, baseURL, token)
	if err != nil {
		return err
	}
	return printDownloadDetail(out, status, jsonOutput)
}

func getDownloadDetails(partialID string, baseURL string, token string) (types.DownloadStatus, error) {
	snapshot, err := readDownloadDetails(context.Background(), partialID, baseURL, token, resolveHostTarget() != "")
	if err != nil {
		return types.DownloadStatus{}, err
	}
	return snapshot.downloads[0], nil
}

func downloadInfoFromStatus(status types.DownloadStatus) downloadInfo {
	status = withoutInactiveRates(status)
	return downloadInfo{
		ID:         status.ID,
		URL:        status.URL,
		Filename:   status.Filename,
		Status:     status.Status,
		Progress:   status.Progress,
		TotalSize:  status.TotalSize,
		Downloaded: status.Downloaded,
		Speed:      status.Speed,
	}
}

func statusFromRecord(record types.DownloadRecord) types.DownloadStatus {
	progress := 0.0
	if record.TotalSize > 0 {
		progress = float64(record.Downloaded) * 100 / float64(record.TotalSize)
	} else if record.Status == "completed" {
		progress = 100
	}

	return types.DownloadStatus{
		ID:           record.ID,
		URL:          record.URL,
		Filename:     record.Filename,
		DestPath:     record.DestPath,
		Status:       record.Status,
		Error:        record.Error,
		TotalSize:    record.TotalSize,
		Downloaded:   record.Downloaded,
		Progress:     progress,
		Speed:        0,
		AddedAt:      record.CreatedAt,
		TimeTaken:    record.TimeTaken,
		AvgSpeed:     record.AvgSpeed,
		RateLimit:    record.RateLimit,
		RateLimitSet: record.RateLimitSet,
	}
}

func truncateFilenameForDisplay(filename string, maxRunes int) string {
	if utf8.RuneCountInString(filename) <= maxRunes {
		return filename
	}
	return string([]rune(filename)[:maxRunes-3]) + "..."
}

func printDownloadDetail(out io.Writer, d types.DownloadStatus, jsonOutput bool) error {
	d = withoutInactiveRates(d)
	if jsonOutput {
		return clioutput.WriteJSON(out, d)
	}

	var text strings.Builder
	fmt.Fprintf(&text, "ID:         %s\n", safeTerminalText(d.ID))
	fmt.Fprintf(&text, "URL:        %s\n", safeTerminalText(d.URL))
	fmt.Fprintf(&text, "Filename:   %s\n", safeTerminalText(d.Filename))
	fmt.Fprintf(&text, "Status:     %s\n", safeTerminalText(d.Status))
	fmt.Fprintf(&text, "Progress:   %.1f%%\n", d.Progress)
	fmt.Fprintf(&text, "Downloaded: %s / %s\n", utils.FormatBytes(d.Downloaded), utils.FormatBytes(d.TotalSize))
	if d.Speed > 0 {
		fmt.Fprintf(&text, "Speed:      %s\n", utils.FormatSpeed(d.Speed))
	}
	if d.AvgSpeed > 0 {
		fmt.Fprintf(&text, "Average:    %s\n", utils.FormatSpeed(d.AvgSpeed))
	}
	if d.Error != "" {
		fmt.Fprintf(&text, "Error:      %s\n", safeTerminalText(d.Error))
	}
	return clioutput.WriteText(out, text.String())
}

func init() {
	rootCmd.AddCommand(lsCmd)
}
