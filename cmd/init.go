package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new creed project",
	Long:  `Creates a creed.yaml config file and a .creed/ source directory in the current project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("creed init — not yet implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
