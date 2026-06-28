package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Emit context files for configured targets",
	Long: `Reads the creed configuration and generates the appropriate context files
(AgENTS.md, CLAUDE.md, .cursor/rules/, etc.) for each configured target.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := getTarget(cmd)
		if err != nil {
			return err
		}
		if target != "" {
			fmt.Printf("creed sync --target %s — not yet implemented\n", target)
		} else {
			fmt.Println("creed sync — not yet implemented")
		}
		return nil
	},
}

func init() {
	addTargetFlag(syncCmd)
	rootCmd.AddCommand(syncCmd)
}
