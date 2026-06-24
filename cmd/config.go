package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/tui/colors"
	"github.com/charmbracelet/x/term"
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
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return config.GetSettingPaths(), cobra.ShellCompDirectiveNoFileComp
		}
		if len(args) == 1 {
			if _, _, err := config.ParseConfigPath(args[0]); err == nil {
				return []string{"default"}, cobra.ShellCompDirectiveNoFileComp
			}
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := config.LoadSettings()
		if err != nil {
			return fmt.Errorf("failed to load settings: %w", err)
		}

		if len(args) > 0 && args[0] == "open" {
			return openSettingsFile()
		}

		if len(args) > 0 && args[0] == "help" {
			return cmd.Help()
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

	width, _, err := term.GetSize(uintptr(os.Stdout.Fd()))
	if err != nil || width < 1 {
		width = 100
	}

	headerStyle := lipgloss.NewStyle().Foreground(colors.Magenta()).Bold(true).Padding(0, 1)
	nameStyle := lipgloss.NewStyle().Foreground(colors.White()).Padding(0, 1)
	valueStyle := lipgloss.NewStyle().Foreground(colors.LightGray()).Padding(0, 1)
	borderStyle := lipgloss.NewStyle().Foreground(colors.LightGray())

	foundAny := false

	for _, cat := range settings.CategoriesList {
		if cat.Name == "Categories" {
			set := settings.FindSetting("Categories", "category_enabled")
			if set != nil && matchesSearch(cat.Name, cat.Name+"."+set.Key, set.Description, terms) {
				t := buildCategoryTable(cat.Name, width, headerStyle, nameStyle, valueStyle, borderStyle)
				t.Row(set.Key, formatValue(set))
				cmd.Println(t.Render())
				foundAny = true
			}
			continue
		}

		t := buildCategoryTable(cat.Name, width, headerStyle, nameStyle, valueStyle, borderStyle)
		hasAnyRow := false
		for _, set := range cat.Settings {
			pathStr := fmt.Sprintf("%s.%s", cat.Name, set.Key)
			if matchesSearch(cat.Name, pathStr, set.Description, terms) {
				t.Row(set.Key, formatValue(set))
				hasAnyRow = true
			}
		}
		if hasAnyRow {
			cmd.Println(t.Render())
			foundAny = true
		}
	}

	if !foundAny && len(terms) > 0 {
		cmd.Printf("No settings found matching your search.\n")
	}
	return nil
}

func buildCategoryTable(name string, width int, headerStyle, nameStyle, valueStyle, borderStyle lipgloss.Style) *table.Table {
	return table.New().
		Headers(name, "").
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
}

func formatValue(set *config.Setting) string {
	if set.Type == config.TypeDuration {
		switch v := set.Value.(type) {
		case int64:
			return time.Duration(v).String()
		case float64:
			return time.Duration(int64(v)).String()
		case int:
			return time.Duration(int64(v)).String()
		}
	}
	return fmt.Sprintf("%v", set.Value)
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
