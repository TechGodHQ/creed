package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
)

var (
	syncDryRun bool
	syncForce  bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Emit context files for configured targets",
	Long: `Reads .creed/manifest.yaml and generates the appropriate context files
(AGENTS.md, CLAUDE.md, .cursor/rules/, etc.) for each configured target.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := getTarget(cmd)
		if err != nil {
			return err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		result, err := service.New(cwd).Sync(cmd.Context(), usecase.SyncOptions{
			Target: target,
			DryRun: syncDryRun,
			Force:  syncForce,
		})
		if err != nil {
			return err
		}
		for _, targetResult := range result.Targets {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d written, %d skipped, %d failed\n",
				targetResult.Target,
				targetResult.FilesWritten,
				targetResult.FilesSkipped,
				targetResult.FilesFailed,
			)
		}
		if result.HasErrors() {
			return fmt.Errorf("sync completed with errors")
		}
		return nil
	},
}

func init() {
	addTargetFlag(syncCmd)
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "show files that would be emitted without writing")
	syncCmd.Flags().BoolVar(&syncForce, "force", false, "rewrite files even when content is unchanged")
	rootCmd.AddCommand(syncCmd)
}
