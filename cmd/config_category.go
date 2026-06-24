package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

var categoryCmd = &cobra.Command{
	Use:   "category [name]",
	Short: "Manage custom download categories",
	Long:  "Manage custom download categories used for sorting downloads.\n\nSubcommands:\n  list    List all custom categories\n  add     Add a new category\n  remove  Remove a category\n\nRun 'surge category <name>' to view details for a specific category.",
	Args:  cobra.MaximumNArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return config.GetCategoryNames(), cobra.ShellCompDirectiveNoFileComp
	},
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

		width, _, err := term.GetSize(uintptr(os.Stdout.Fd()))
		if err != nil || width < 1 {
			width = 100
		}

		headerStyle := lipgloss.NewStyle().Foreground(colors.Magenta()).Bold(true).Padding(0, 1)
		nameStyle := lipgloss.NewStyle().Foreground(colors.White()).Padding(0, 1)
		valueStyle := lipgloss.NewStyle().Foreground(colors.LightGray()).Padding(0, 1)
		borderStyle := lipgloss.NewStyle().Foreground(colors.LightGray())

		t := table.New().
			Headers("Name", "Description", "Pattern", "Path").
			Width(width).
			BorderBottom(true).
			BorderHeader(true).
			BorderTop(false).
			BorderLeft(false).
			BorderRight(false).
			BorderColumn(false).
			BorderRow(false).
			BorderStyle(borderStyle).
			StyleFunc(func(row, col int) lipgloss.Style {
				switch {
				case row == table.HeaderRow:
					return headerStyle
				case col == 0:
					return nameStyle
				default:
					return valueStyle
				}
			})

		for _, cat := range settings.Categories.Categories {
			t.Row(cat.Name, cat.Description, cat.Pattern, cat.Path)
		}
		cmd.Println(t.Render())
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
			if errors.Is(err, config.ErrCategoryExists) {
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
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return config.GetCategoryNames(), cobra.ShellCompDirectiveNoFileComp
	},
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
	rootCmd.AddCommand(categoryCmd)
	categoryCmd.AddCommand(categoryListCmd)
	categoryCmd.AddCommand(categoryAddCmd)
	categoryCmd.AddCommand(categoryRemoveCmd)
}
