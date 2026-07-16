package cmd

import (
	"github.com/spf13/cobra"

	"github.com/techgodhq/creed/internal/service"
)

const version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:     "creed",
	Short:   "One source of truth for AI context",
	Long:    "creed syncs skills, specs, and config across AGENTS.md, CLAUDE.md, .cursor/rules, and more.",
	Version: version,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	rootCmd.SetVersionTemplate("creed {{.Version}}\n")
	svc := service.New(".")
	registerGeneratedCommands(rootCmd, svc)
	rootCmd.AddCommand(newMCPCommand(".", serveMCPStdio))
}
