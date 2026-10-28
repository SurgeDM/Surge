package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/store"
	"github.com/spf13/cobra"
)

type diagnosticCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
	ID      string `json:"id,omitempty"`
}
type doctorReport struct {
	SchemaVersion int               `json:"schema_version"`
	OK            bool              `json:"ok"`
	Scope         string            `json:"scope"`
	Checks        []diagnosticCheck `json:"checks"`
}

func localDoctor() doctorReport {
	report := doctorReport{SchemaVersion: 1, OK: true, Scope: "local", Checks: make([]diagnosticCheck, 0)}
	settings, _, err := config.InspectSettings(config.GetSettingsPath())
	if err != nil {
		report.OK = false
		report.Checks = append(report.Checks, diagnosticCheck{Name: "config", Status: "error", Message: "Configuration is invalid or unreadable; run surge config validate."})
	} else {
		report.Checks = append(report.Checks, diagnosticCheck{Name: "config", Status: "ok", Message: "Configuration is valid; omitted settings use defaults."})
		destination := config.Resolve[string](settings.General.DefaultDownloadDir)
		if destination != "" {
			info, err := os.Stat(destination)
			if err != nil || !info.IsDir() {
				report.Checks = append(report.Checks, diagnosticCheck{Name: "destination", Status: "warning", Message: "Default destination is missing or inaccessible; choose an existing output directory."})
			}
		}
	}
	for _, location := range []struct{ name, path string }{{"config_directory", config.GetSurgeDir()}, {"state_directory", config.GetStateDir()}, {"runtime_directory", config.GetRuntimeDir()}} {
		info, err := os.Stat(location.path)
		status, message := "ok", "Directory is accessible."
		if errors.Is(err, os.ErrNotExist) {
			status, message = "info", "Directory does not exist yet; it will be created when needed."
		} else if err != nil || !info.IsDir() {
			status, message = "error", "Path is inaccessible or is not a directory; check permissions."
			report.OK = false
		}
		report.Checks = append(report.Checks, diagnosticCheck{Name: location.name, Status: status, Message: message})
	}
	records, err := store.InspectDirectory(config.GetStateDir())
	if err != nil {
		report.OK = false
		report.Checks = append(report.Checks, diagnosticCheck{Name: "state", Status: "error", Message: "Saved state is unreadable or unsupported; preserve it before investigating."})
		return report
	}
	inspected := 0
	for _, record := range records {
		if record.Status != "paused" && record.Status != "downloading" && record.Status != "queued" {
			continue
		}
		if inspected >= 256 {
			report.Checks = append(report.Checks, diagnosticCheck{Name: "resume", Status: "info", Message: "Resume-file inspection limited to the first 256 unfinished downloads."})
			break
		}
		inspected++
		// No source requests, file contents, repairs, orphan cleanup, or validator changes.
		if record.DestPath == "" {
			continue
		}
		info, err := os.Stat(record.DestPath)
		if err != nil || info.IsDir() {
			report.Checks = append(report.Checks, diagnosticCheck{Name: "resume_file", Status: "warning", ID: record.ID, Message: "Working file is missing or inaccessible; inspect this download before resuming."})
		}
	}
	report.Checks = append(report.Checks, diagnosticCheck{Name: "state", Status: "ok", Message: fmt.Sprintf("Read %d saved records; inspected %d unfinished file paths. No files were changed.", len(records), inspected)})
	return report
}
func newDoctorCommand() *cobra.Command {
	command := &cobra.Command{Use: "doctor", Short: "Inspect local health without repairs; optionally check the server", Args: cobra.NoArgs, Annotations: map[string]string{"surge.contract": "1"}, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := validateOutputVersion(cmd); err != nil {
			return err
		}
		remote, _ := cmd.Flags().GetBool("server")
		if !remote && resolveHostTarget() != "" {
			return usageError("Use --server to check the selected host, or omit --host and SURGE_HOST for local diagnostics.")
		}
		report := doctorReport{SchemaVersion: 1, OK: true, Scope: "server", Checks: make([]diagnosticCheck, 0)}
		if remote {
			client, url, token, err := lifecycleConnection()
			if err != nil {
				return err
			}
			defer client.CloseIdleConnections()
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			// Capabilities is authenticated by the daemon; health alone may be public.
			if _, err := readServerCapabilities(ctx, client, url, token); err != nil {
				return err
			}
			response, err := commandAPIRequest(ctx, client, http.MethodGet, url, token, "/health")
			if err != nil {
				return err
			}
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return remoteStatusError(response.StatusCode, clioutput.NewError(clioutput.Unavailable, "Server health check failed.", nil))
			}
			report.Checks = append(report.Checks, diagnosticCheck{Name: "server", Status: "ok", Message: "Authenticated capabilities and health requests succeeded."})
		} else {
			report = localDoctor()
		}
		var failure error
		if !report.OK {
			failure = clioutput.NewError(clioutput.OperationFailed, "Diagnostics found a problem; inspect the checks for recovery steps.", nil)
		}
		jsonOutput, _ := cmd.Flags().GetBool("json")
		version, _ := cmd.Flags().GetString("output-version")
		if jsonOutput || version == "1" {
			if failure != nil {
				return &commandResultError{result: report, cause: failure}
			}
			return clioutput.WriteJSON(cmd.OutOrStdout(), report)
		}
		var text strings.Builder
		for _, check := range report.Checks {
			fmt.Fprintf(&text, "%s: %s - %s\n", check.Name, check.Status, check.Message)
		}
		if err := clioutput.WriteText(cmd.OutOrStdout(), text.String()); err != nil {
			return err
		}
		return failure
	}}
	command.Flags().Bool("server", false, "Explicitly check server authentication and health (five-second limit)")
	command.Flags().Bool("json", false, "Write redacted version 1 diagnostics")
	command.Flags().String("output-version", "", "Supported output version: 1")
	return command
}
func init() { rootCmd.AddCommand(newDoctorCommand()) }
