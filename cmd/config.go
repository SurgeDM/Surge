package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

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
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := config.LoadSettings()
		if err != nil {
			return fmt.Errorf("failed to load settings: %w", err)
		}

		if len(args) > 0 && args[0] == "open" {
			return openSettingsFile()
		}

		isSetOperation := len(args) >= 2 && strings.Contains(args[0], ".")
		if isSetOperation {
			return handleSetOperation(cmd, settings, args)
		}

		return handleSearchOperation(cmd, settings, args)
	},
}

func handleSearchOperation(cmd *cobra.Command, settings *config.Settings, args []string) error {
	terms := make([]string, 0, len(args))
	for _, arg := range args {
		terms = append(terms, strings.ToLower(arg))
	}

	if len(terms) > 0 {
		cmd.Printf("Search Results:\n\n")
	} else {
		cmd.Printf("Available Surge Settings:\n\n")
	}

	foundAny := false
	for _, cat := range settings.CategoriesList {
		if cat.Name == "Categories" {
			set := settings.FindSetting("Categories", "category_enabled")
			if set != nil {
				pathStr := "Categories.category_enabled"
				if matchesSearch(cat.Name, pathStr, set.Description, terms) {
					printSetting(cmd, cat.Name, pathStr, set)
					foundAny = true
				}
			}
			continue
		}

		var matchingSets []*config.Setting
		for _, set := range cat.Settings {
			pathStr := fmt.Sprintf("%s.%s", cat.Name, set.Key)
			if matchesSearch(cat.Name, pathStr, set.Description, terms) {
				matchingSets = append(matchingSets, set)
			}
		}

		if len(matchingSets) > 0 {
			cmd.Printf("[%s]\n", cat.Name)
			for _, set := range matchingSets {
				pathStr := fmt.Sprintf("%s.%s", cat.Name, set.Key)
				cmd.Printf("  %-32s : %v\n", pathStr, set.Value)
				if set.Description != "" {
					cmd.Printf("      %s\n", set.Description)
				}
			}
			cmd.Println()
			foundAny = true
		}
	}

	if !foundAny && len(terms) > 0 {
		cmd.Printf("No settings found matching your search.\n")
	}
	return nil
}

func printSetting(cmd *cobra.Command, catName, pathStr string, set *config.Setting) {
	cmd.Printf("[%s]\n", catName)
	cmd.Printf("  %-32s : %v\n", pathStr, set.Value)
	if set.Description != "" {
		cmd.Printf("      %s\n", set.Description)
	}
	cmd.Println()
}

func handleSetOperation(cmd *cobra.Command, settings *config.Settings, args []string) error {
	if len(args) > 2 {
		return fmt.Errorf("too many arguments for config set operation")
	}

	path := args[0]
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

	if err := config.SaveSettings(settings); err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}

	return nil
}

func matchesSearch(catName, pathStr, desc string, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	searchTarget := strings.ToLower(fmt.Sprintf("%s %s %s", catName, pathStr, desc))
	for _, term := range terms {
		if !strings.Contains(searchTarget, term) {
			return false
		}
	}
	return true
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
