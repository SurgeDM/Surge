package cmd

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/spf13/cobra"
)

// commandResultError carries receipts from an operation that partially failed.
// The process boundary writes this result once and preserves its exit status.
type commandResultError struct {
	result any
	cause  error
}

func (e *commandResultError) Error() string { return e.cause.Error() }
func (e *commandResultError) Unwrap() error { return e.cause }

// Run is the process boundary: commands return errors; this function reports
// them once and selects the exit status. The caller owns process termination.
func Run(args []string, stdout, stderr io.Writer) int {
	return runCommand(rootCmd, args, stdout, stderr)
}

// Stderr diagnostics are best effort: a failed diagnostic write cannot be
// reported to another stream without corrupting stdout, and we already fail.
func runCommand(root *cobra.Command, args []string, stdout, stderr io.Writer) int {
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	command, err := root.ExecuteC()
	if err == nil {
		return 0
	}

	// Flag parsing may fail before a version is selected. In that case retain
	// the legacy stderr diagnostic rather than guessing a format from argv.
	version := ""
	versionSelected := false
	if command != nil {
		if flag := command.Flags().Lookup("output-version"); flag != nil {
			version = flag.Value.String()
			versionSelected = flag.Changed
		}
	}

	// wait is a new command: its --json flag selects the v1 contract even
	// when argument validation fails before RunE.
	if !versionSelected && version == "" && command != nil && command.Name() == "wait" {
		if jsonOutput, _ := command.Flags().GetBool("json"); jsonOutput {
			version = "1"
		}
	}

	var writeError *clioutput.WriteError
	if errors.As(err, &writeError) {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	if version == "1" {
		result := any(clioutput.Failed(command.Name(), err))
		var resultError *commandResultError
		if errors.As(err, &resultError) {
			result = resultError.result
		}
		if writeErr := clioutput.WriteJSON(stdout, result); writeErr != nil {
			_, _ = fmt.Fprintln(stderr, writeErr)
			return 1
		}
		return clioutput.ExitCode(err)
	}
	_, _ = fmt.Fprintln(stderr, err)
	if command != nil && command.Name() == "wait" {
		return clioutput.ExitCode(err)
	}
	if versionSelected {
		return clioutput.ExitCode(err)
	}
	return 1
}

// remoteStatusError keeps legacy diagnostics while classifying responses for
// the CLI contract. Never include a response body in its public message.
func remoteStatusError(status int, cause error) error {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return clioutput.NewError(clioutput.Unauthorized, "The Surge server denied access.", cause)
	case http.StatusNotFound:
		return clioutput.NewError(clioutput.NotFound, "The requested resource was not found on the Surge server.", cause)
	case http.StatusConflict:
		return clioutput.NewError(clioutput.Conflict, "The request conflicts with the current server state.", cause)
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return clioutput.NewError(clioutput.Timeout, "The Surge server request timed out.", cause)
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable:
		return clioutput.NewError(clioutput.Unavailable, "The Surge server is temporarily unavailable.", cause)
	default:
		return cause
	}
}

// downloadItem is a CLI DTO, intentionally independent of engine/API structs.
// Byte counts are decimal strings to preserve int64 precision in JSON clients.
// Unknown totals/rates are null. Source URLs and raw error strings are excluded
// because either can contain credentials or signed query parameters.
type downloadItem struct {
	ID                  string   `json:"id"`
	Filename            string   `json:"filename"`
	Status              string   `json:"status"`
	DownloadedBytes     string   `json:"downloaded_bytes"`
	TotalBytes          *string  `json:"total_bytes"`
	SpeedBytesPerSecond *float64 `json:"speed_bytes_per_second"`
}

func downloadItems(statuses []types.DownloadStatus) []downloadItem {
	items := make([]downloadItem, 0, len(statuses))
	for _, status := range statuses {
		item := downloadItem{ID: status.ID, Filename: status.Filename, Status: status.Status, DownloadedBytes: strconv.FormatInt(status.Downloaded, 10)}
		if status.TotalSize > 0 {
			total := strconv.FormatInt(status.TotalSize, 10)
			item.TotalBytes = &total
		}
		if status.Status == "downloading" && status.Speed >= 0 && !math.IsNaN(status.Speed) && !math.IsInf(status.Speed, 0) {
			speed := status.Speed
			item.SpeedBytesPerSecond = &speed
		}
		items = append(items, item)
	}
	return items
}
