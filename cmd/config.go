package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config [path] [value]",
	Short: "Manage application settings",
	Long: `Manage Surge application settings.

Provides a command-line interface to get, set, or reset settings in the configuration file.
To list all available settings or view documentation, refer to the UI or documentation.

Usage:
  surge config General.Auto_Resume true     (Sets a value)
  surge config Network.Max_Concurrent_Downloads (Gets a value)
  surge config Performance.Stall_Timeout default (Resets to default)
  surge config open                         (Opens settings file in editor)`,
	Args: cobra.RangeArgs(0, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := config.LoadSettings()
		if err != nil {
			return fmt.Errorf("failed to load settings: %w", err)
		}

		if len(args) == 0 {
			cmd.Printf("Available Surge Settings:\n\n")
			for _, cat := range settings.CategoriesList {
				// Don't clutter with Categories struct unless needed, but it's ok to list everything
				if cat.Name == "Categories" {
					cmd.Printf("[%s]\n", cat.Name)
					set := settings.FindSetting("Categories", "category_enabled")
					if set != nil {
						cmd.Printf("  %-32s : %v\n", "Categories.category_enabled", set.Value)
						cmd.Printf("      %s\n", set.Description)
					}
					cmd.Println()
					continue
				}

				cmd.Printf("[%s]\n", cat.Name)
				for _, set := range cat.Settings {
					pathStr := fmt.Sprintf("%s.%s", cat.Name, set.Key)
					cmd.Printf("  %-32s : %v\n", pathStr, set.Value)
					if set.Description != "" {
						cmd.Printf("      %s\n", set.Description)
					}
				}
				cmd.Println()
			}
			return nil
		}

		path := args[0]

		if path == "open" {
			return openSettingsFile()
		}

		// GET operation
		if len(args) == 1 {
			set, err := config.GetSetting(settings, path)
			if err != nil {
				return err
			}
			cmd.Printf("%s\n", set.Description)
			cmd.Printf("Current Value: %v\n", set.Value)
			return nil
		}

		// SET or RESET operation
		value := args[1]
		if value == "default" {
			if err := config.ResetSetting(settings, path); err != nil {
				return err
			}
			cmd.Printf("Reset %s to default value.\n", path)
		} else {
			if err := config.SetSetting(settings, path, value); err != nil {
				return err
			}
			cmd.Printf("Set %s to %s\n", path, value)
		}

		// Save the modified settings
		if err := config.SaveSettings(settings); err != nil {
			return fmt.Errorf("failed to save settings: %w", err)
		}

		return nil
	},
}

func init() {
	configCmd.AddCommand(categoryCmd)
	rootCmd.AddCommand(configCmd)
}

func openSettingsFile() error {
	path := config.GetSettingsPath()

	var command string
	var args []string

	if editor := os.Getenv("EDITOR"); editor != "" {
		command = editor
		args = []string{path}
	} else {
		switch runtime.GOOS {
		case "windows":
			command = "cmd"
			args = []string{"/c", "start", "", path}
		case "darwin":
			command = "open"
			args = []string{path}
		default: // linux, bsd, etc
			command = "xdg-open"
			args = []string{path}
		}
	}

	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}
	return nil
}
