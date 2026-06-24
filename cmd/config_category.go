package cmd

import (
	"fmt"
	"strings"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/spf13/cobra"
)

var categoryCmd = &cobra.Command{
	Use:   "category [name]",
	Short: "Manage custom download categories",
	Long:  "List, add, remove, or view custom download categories used for sorting downloads.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := config.LoadSettings()
		if err != nil {
			return fmt.Errorf("failed to load settings: %w", err)
		}

		if len(args) == 0 {
			// Without args, list the category settings
			enabledSet := settings.FindSetting("Categories", "category_enabled")
			if enabledSet != nil {
				cmd.Printf("Category Routing Enabled: %v\n\n", enabledSet.Value)
			}
			return cmd.Help()
		}

		// Show specific category details
		name := args[0]
		for _, cat := range settings.Categories.Categories {
			if strings.EqualFold(cat.Name, name) {
				cmd.Printf("Category: %s\n", cat.Name)
				cmd.Printf("  Description: %s\n", cat.Description)
				cmd.Printf("  Pattern:     %s\n", cat.Pattern)
				cmd.Printf("  Path:        %s\n", cat.Path)
				return nil
			}
		}
		return fmt.Errorf("category '%s' not found", name)
	},
}

var categoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all custom categories",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := config.LoadSettings()
		if err != nil {
			return err
		}

		if len(settings.Categories.Categories) == 0 {
			cmd.Println("No custom categories configured.")
			return nil
		}

		cmd.Printf("%-20s %-30s %s\n", "NAME", "PATTERN", "PATH")
		cmd.Printf("%-20s %-30s %s\n", strings.Repeat("-", 20), strings.Repeat("-", 30), strings.Repeat("-", 20))
		for _, cat := range settings.Categories.Categories {
			cmd.Printf("%-20s %-30s %s\n", cat.Name, cat.Pattern, cat.Path)
		}
		return nil
	},
}

var categoryAddCmd = &cobra.Command{
	Use:   "add <name> <pattern> <path>",
	Short: "Add a new custom category",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		pattern := args[1]
		path := args[2]

		settings, err := config.LoadSettings()
		if err != nil {
			return err
		}

		err = settings.AddCategory(name, pattern, path)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				if updateErr := settings.UpdateCategory(name, pattern, path); updateErr != nil {
					return updateErr
				}
				cmd.Printf("Updated existing category '%s'.\n", name)
				return nil
			}
			return err
		}
		cmd.Printf("Added new category '%s'.\n", name)
		return nil
	},
}

var categoryRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a custom category",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		settings, err := config.LoadSettings()
		if err != nil {
			return err
		}

		if err := settings.RemoveCategory(name); err != nil {
			return err
		}
		cmd.Printf("Removed category '%s'.\n", name)
		return nil
	},
}

func init() {
	categoryCmd.AddCommand(categoryListCmd)
	categoryCmd.AddCommand(categoryAddCmd)
	categoryCmd.AddCommand(categoryRemoveCmd)
}
