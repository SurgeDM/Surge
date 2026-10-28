package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/spf13/cobra"
)

type capabilityReport struct {
	SchemaVersion int                 `json:"schema_version"`
	Version       string              `json:"version"`
	Commands      []string            `json:"commands"`
	Schemas       map[string]int      `json:"schemas"`
	Limits        map[string]int      `json:"limits"`
	Units         map[string]string   `json:"units"`
	Features      map[string]bool     `json:"features"`
	Server        *serverCapabilities `json:"server,omitempty"`
}

func offlineCapabilities() capabilityReport {
	return capabilityReport{SchemaVersion: 1, Version: Version, Commands: []string{"add", "fetch", "ls", "wait", "pause", "resume", "cancel", "capabilities"}, Schemas: map[string]int{"add": 1, "fetch": 1, "ls": 1, "wait": 1, "lifecycle": 1, "manifest": 1}, Limits: map[string]int{"manifest_bytes": maxManifestBytes, "manifest_items": maxWaitIDs, "wait_ids": maxWaitIDs, "idempotency_key_bytes": 128}, Units: map[string]string{"bytes": "decimal string", "speed": "bytes per second", "duration": "seconds", "unknown": "null"}, Features: map[string]bool{"foreground": true, "sha256": true, "safe_destination": true, "manifests": true, "idempotency_remote": true}}
}
func newCapabilitiesCommand() *cobra.Command {
	command := &cobra.Command{Use: "capabilities", Short: "Discover offline contracts and optional server features", Args: cobra.NoArgs, Annotations: map[string]string{"surge.contract": "1"}, RunE: func(cmd *cobra.Command, _ []string) error {
		if err := validateOutputVersion(cmd); err != nil {
			return err
		}
		report := offlineCapabilities()
		server, _ := cmd.Flags().GetBool("server")
		if server {
			client, url, token, err := lifecycleConnection()
			if err != nil {
				return err
			}
			defer client.CloseIdleConnections()
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			caps, err := readServerCapabilities(ctx, client, url, token)
			if err != nil {
				return err
			}
			report.Server = &caps
		}
		jsonOutput, _ := cmd.Flags().GetBool("json")
		version, _ := cmd.Flags().GetString("output-version")
		if jsonOutput || version == "1" {
			return clioutput.WriteJSON(cmd.OutOrStdout(), report)
		}
		return clioutput.WriteText(cmd.OutOrStdout(), fmt.Sprintf("Surge %s: output schema 1; manifest schema 1; %d items / %d bytes maximum.\nUse --json for contracts and --server to negotiate daemon features.\n", Version, maxWaitIDs, maxManifestBytes))
	}}
	command.Flags().Bool("server", false, "Explicitly query the configured server with a five-second limit")
	command.Flags().Bool("json", false, "Write machine-readable capabilities")
	command.Flags().String("output-version", "", "Supported output version: 1")
	return command
}
func init() { rootCmd.AddCommand(newCapabilitiesCommand()) }
