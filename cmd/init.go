package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/techgodhq/creed/internal/service"
)

var initCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Initialize a new creed project",
	Long:  `Creates a .creed/ source directory and manifest.yaml in the current project.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := ""
		if len(args) > 0 {
			projectName = args[0]
		}
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		if err := service.New(cwd).Init(cmd.Context(), projectName); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Initialized creed project")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
