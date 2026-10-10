package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"github.com/SurgeDM/Surge/internal/store"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:     "ls [id]",
	Aliases: []string{"l"},
	Short:   "List downloads",
	Long:    `List all downloads from the running server or database. Optionally show details for a specific download by ID.`,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := initializeGlobalState(); err != nil {
			return err
		}

		jsonOutput, _ := cmd.Flags().GetBool("json")
		watch, _ := cmd.Flags().GetBool("watch")
		if err := validateLSFlags(len(args), jsonOutput, watch); err != nil {
			return err
		}

		baseURL, token, err := resolveAPIConnection(false)
		if err != nil {
			return err
		}

		// If ID provided, show details for that download
		if len(args) == 1 {
			return showDownloadDetails(args[0], jsonOutput, baseURL, token)
		}

		strictRemote := resolveHostTarget() != ""

		if watch {
			for {
				// Clear screen first for watch mode
				fmt.Print("\033[H\033[2J")
				if err := printDownloads(jsonOutput, baseURL, token, strictRemote); err != nil {
					return err
				}
				time.Sleep(1 * time.Second)
			}
		}
		return printDownloads(jsonOutput, baseURL, token, strictRemote)
	},
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

func printDownloads(jsonOutput bool, baseURL string, token string, strictRemote bool) error {
	var downloads []downloadInfo

	// Try to get from running server first
	if baseURL != "" {
		serverDownloads, err := GetRemoteDownloads(baseURL, token)
		if err != nil {
			if strictRemote {
				return fmt.Errorf("error listing remote downloads: %w", err)
			}
		} else {
			for _, status := range serverDownloads {
				downloads = append(downloads, downloadInfoFromStatus(status))
			}
		}
	}

	// Fall back to database only when not explicitly targeting a remote host.
	if len(downloads) == 0 && (!strictRemote || baseURL == "") {
		dbDownloads, err := store.ListAllDownloads()
		if err != nil {
			return fmt.Errorf("error listing downloads: %w", err)
		}

		for _, download := range dbDownloads {
			downloads = append(downloads, downloadInfoFromStatus(statusFromRecord(download)))
		}
	}

	if len(downloads) == 0 {
		if !jsonOutput {
			fmt.Println("No downloads found.")
		} else {
			fmt.Println("[]")
		}
		return nil
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(downloads, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Table output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
		id := d.ID
		if len(id) > 8 {
			id = id[:8]
		}

		// Truncate filename
		filename := truncateFilenameForDisplay(d.Filename, 25)

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", id, filename, d.Status, progress, speed, size)
	}
	_ = w.Flush()
	return nil
}

func showDownloadDetails(partialID string, jsonOutput bool, baseURL string, token string) error {
	strictRemote := resolveHostTarget() != ""

	// Resolve partial ID
	fullID, err := resolveDownloadID(partialID)
	if err != nil {
		return err
	}

	// Try to get from running server first
	if baseURL != "" {
		path := fmt.Sprintf("/download?id=%s", url.QueryEscape(fullID))
		resp, err := doAPIRequest(http.MethodGet, baseURL, token, path, nil)
		if err != nil {
			if strictRemote {
				return fmt.Errorf("error fetching remote download details: %w", err)
			}
		} else {
			defer func() {
				if err := resp.Body.Close(); err != nil {
					utils.Debug("Error closing response body: %v", err)
				}
			}()
			if resp.StatusCode == http.StatusOK {
				var status types.DownloadStatus
				if err := json.NewDecoder(resp.Body).Decode(&status); err == nil {
					printDownloadDetail(status, jsonOutput)
					return nil
				} else if strictRemote {
					return fmt.Errorf("error decoding remote download details: %w", err)
				}
			} else if strictRemote {
				if resp.StatusCode == http.StatusNotFound {
					return fmt.Errorf("remote download not found: %s", partialID)
				}
				return fmt.Errorf("remote server returned %s", resp.Status)
			}
		}
	}

	// Fall back to database - search through all downloads
	downloads, err := store.ListAllDownloads()
	if err != nil {
		return fmt.Errorf("error listing downloads: %w", err)
	}

	var found *types.DownloadRecord
	for _, d := range downloads {
		if d.ID == fullID {
			found = &d
			break
		}
	}

	if found == nil {
		return fmt.Errorf("download not found: %s", partialID)
	}

	printDownloadDetail(statusFromRecord(*found), jsonOutput)
	return nil
}

func downloadInfoFromStatus(status types.DownloadStatus) downloadInfo {
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
		Speed:        record.AvgSpeed,
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

func printDownloadDetail(d types.DownloadStatus, jsonOutput bool) {
	if jsonOutput {
		data, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("ID:         %s\n", d.ID)
	fmt.Printf("URL:        %s\n", d.URL)
	fmt.Printf("Filename:   %s\n", d.Filename)
	fmt.Printf("Status:     %s\n", d.Status)
	fmt.Printf("Progress:   %.1f%%\n", d.Progress)
	fmt.Printf("Downloaded: %s / %s\n", utils.FormatBytes(d.Downloaded), utils.FormatBytes(d.TotalSize))
	if d.Speed > 0 {
		fmt.Printf("Speed:      %s\n", utils.FormatSpeed(d.Speed))
	}
	if d.Error != "" {
		fmt.Printf("Error:      %s\n", d.Error)
	}
}

func init() {
	rootCmd.AddCommand(lsCmd)
	lsCmd.Flags().Bool("json", false, "Output in JSON format")
	lsCmd.Flags().Bool("watch", false, "Watch mode: refresh every second")
}
