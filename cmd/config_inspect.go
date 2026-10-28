package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/transferui"
	"github.com/spf13/cobra"
)

type configEntry struct {
	Key          string `json:"key"`
	Value        any    `json:"value"`
	Default      any    `json:"default"`
	Source       string `json:"source"`
	Description  string `json:"description"`
	NeedsRestart bool   `json:"needs_restart"`
}

func redactedSetting(setting *config.Setting, value any) any {
	if setting.Type == config.TypeAuthToken {
		return "[redacted]"
	}
	if setting.Key == "proxy_url" {
		raw, _ := value.(string)
		if raw == "" {
			return ""
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[redacted]"
		}
		return parsed.Scheme + "://" + parsed.Host
	}
	return value
}
func newConfigInspectCommand(mode string) *cobra.Command {
	command := &cobra.Command{Use: mode + " [setting.path]", Short: map[string]string{"show": "Show redacted local settings and their sources", "explain": "Explain local settings, defaults, and restart requirements", "validate": "Validate local TOML without changing it"}[mode], Annotations: map[string]string{"surge.contract": "1"}, Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if resolveHostTarget() != "" {
			return usageError("This command inspects local settings; omit --host and SURGE_HOST.")
		}
		if err := validateOutputVersion(cmd); err != nil {
			return err
		}
		settings, sources, err := config.InspectSettings(config.GetSettingsPath())
		if err != nil {
			return clioutput.NewError(clioutput.InvalidArguments, err.Error()+"; correct settings.toml before retrying.", nil)
		}
		entries := make([]configEntry, 0)
		for _, category := range settings.CategoriesList {
			for _, setting := range category.Settings {
				key := category.Name + "." + setting.Key
				if len(args) > 0 && !strings.EqualFold(args[0], key) {
					continue
				}
				source := "default"
				if sources[strings.ToLower(key)] {
					source = "file"
				}
				entries = append(entries, configEntry{Key: key, Value: redactedSetting(setting, setting.Value), Default: redactedSetting(setting, setting.DefaultValue), Source: source, Description: setting.Description, NeedsRestart: setting.NeedsRestart})
			}
		}
		if len(args) > 0 && len(entries) == 0 {
			return usageError("Unknown setting path; use surge config explain for supported paths.")
		}
		jsonOutput, _ := cmd.Flags().GetBool("json")
		version, _ := cmd.Flags().GetString("output-version")
		if jsonOutput || version == "1" {
			return clioutput.WriteJSON(cmd.OutOrStdout(), struct {
				SchemaVersion int           `json:"schema_version"`
				Valid         bool          `json:"valid"`
				Items         []configEntry `json:"items"`
			}{1, true, entries})
		}
		if mode == "validate" {
			return clioutput.WriteText(cmd.OutOrStdout(), "Local configuration is valid. Missing values use defaults.\n")
		}
		var text strings.Builder
		for _, entry := range entries {
			fmt.Fprintf(&text, "%s = %s (source: %s, restart: %t)\n", entry.Key, transferui.SafeText(fmt.Sprint(entry.Value)), entry.Source, entry.NeedsRestart)
			if mode == "explain" {
				fmt.Fprintf(&text, "  %s; default: %s\n", entry.Description, transferui.SafeText(fmt.Sprint(entry.Default)))
			}
		}
		return clioutput.WriteText(cmd.OutOrStdout(), text.String())
	}}
	command.Flags().Bool("json", false, "Write redacted version 1 configuration diagnostics")
	command.Flags().String("output-version", "", "Supported output version: 1")
	command.ValidArgsFunction = func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return config.GetSettingPaths(), cobra.ShellCompDirectiveNoFileComp
	}
	return command
}
func init() {
	for _, mode := range []string{"show", "explain", "validate"} {
		configCmd.AddCommand(newConfigInspectCommand(mode))
	}
}
