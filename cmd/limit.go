package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/SurgeDM/Surge/internal/utils"
	"github.com/spf13/cobra"
)

var limitCmd = &cobra.Command{
	Use:   "limit [--global|--default] <speed> | limit <id> <speed>",
	Short: "Set download speed limits",
	Long: `Set global, default per-download, or per-download speed limits.

Examples:
  surge limit <id> 2MB/s
  surge limit <id> 0
  surge limit <id> -1
  surge limit --global 10MB/s
  surge limit --default 2MB/s`,
	Args: validateLimitArgs,
	RunE: runLimitCommand,
}

func init() {
	rootCmd.AddCommand(limitCmd)
	limitCmd.Flags().Bool("global", false, "Set the global download speed limit")
	limitCmd.Flags().Bool("default", false, "Set the default per-download speed limit")
}

func validateLimitArgs(cmd *cobra.Command, args []string) error {
	globalLimit, _ := cmd.Flags().GetBool("global")
	defaultLimit, _ := cmd.Flags().GetBool("default")

	if globalLimit && defaultLimit {
		return fmt.Errorf("use only one of --global or --default")
	}
	if globalLimit || defaultLimit {
		if len(args) != 1 {
			return fmt.Errorf("provide exactly one speed value with --global or --default")
		}
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("provide a download ID and speed, or use --global/--default")
	}
	return nil
}

func runLimitCommand(cmd *cobra.Command, args []string) error {
	if err := initializeGlobalState(); err != nil {
		return err
	}

	globalLimit, _ := cmd.Flags().GetBool("global")
	defaultLimit, _ := cmd.Flags().GetBool("default")

	speedArg := args[len(args)-1]

	baseURL, token, err := resolveAPIConnection(true)
	if err != nil {
		return fmt.Errorf("failed to connect to Surge server: %w", err)
	}

	path := ""
	success := ""
	rate := int64(0)
	switch {
	case globalLimit:
		rate, err = utils.ParseRateLimit(speedArg)
		if err != nil {
			return err
		}
		if rate == 0 {
			success = "Set global speed limit to \u221E"
		} else {
			success = fmt.Sprintf("Set global speed limit to %s", utils.FormatRateLimit(rate))
		}
		path = fmt.Sprintf("/rate-limit/global?rate=%d", rate)
	case defaultLimit:
		rate, err = utils.ParseRateLimit(speedArg)
		if err != nil {
			return err
		}
		if rate == 0 {
			success = "Set default download speed limit to \u221E"
		} else {
			success = fmt.Sprintf("Set default download speed limit to %s", utils.FormatRateLimit(rate))
		}
		path = fmt.Sprintf("/rate-limit/default?rate=%d", rate)
	default:
		id, err := resolveDownloadID(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve download ID: %w", err)
		}
		speedStr := strings.TrimSpace(speedArg)
		// -1 is used as a numeric alias for "inherit" so users don't have to type a string
		if strings.EqualFold(speedStr, "inherit") || strings.EqualFold(speedStr, "default") || speedStr == "-1" {
			path = fmt.Sprintf("/rate-limit?id=%s&inherit=true", url.QueryEscape(id))
			success = fmt.Sprintf("Set speed limit for %s to inherit the default", id)
		} else {
			rate, err = utils.ParseRateLimit(speedArg)
			if err != nil {
				return err
			}
			path = fmt.Sprintf("/rate-limit?id=%s&rate=%d", url.QueryEscape(id), rate)
			if rate == 0 {
				success = fmt.Sprintf("Set speed limit for %s to \u221E", id)
			} else {
				success = fmt.Sprintf("Set speed limit for %s to %s", id, utils.FormatRateLimit(rate))
			}
		}
	}

	if err := executeLimitRequest(baseURL, token, path); err != nil {
		return err
	}

	fmt.Println(success)
	return nil
}

func executeLimitRequest(baseURL, token, path string) error {
	resp, err := doAPIRequest(http.MethodPost, baseURL, token, path, nil)
	if err != nil {
		return fmt.Errorf("failed to send request to server: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			utils.Debug("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server error: %s - %s", resp.Status, string(body))
	}
	return nil
}
