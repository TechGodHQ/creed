package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const version = "0.2.0"

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
}

// addTargetFlag adds a --target flag to the given command.
func addTargetFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("target", "t", "", "emit for a specific target (claude, cursor, codex, windsurf, aider)")
}

// getTarget returns the target flag value, or empty string for "all".
func getTarget(cmd *cobra.Command) (string, error) {
	target, err := cmd.Flags().GetString("target")
	if err != nil {
		return "", fmt.Errorf("failed to read --target flag: %w", err)
	}
	return target, nil
}
