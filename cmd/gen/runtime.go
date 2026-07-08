package gen

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
)

func runInit(cmd *cobra.Command, s service.Service, args []string) error {
	projectName := ""
	if len(args) > 0 {
		projectName = args[0]
	}
	if err := s.Init(cmd.Context(), projectName); err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Initialized creed project")
	return nil
}

func runSync(cmd *cobra.Command, s service.Service, _ []string) error {
	target, err := cmd.Flags().GetString("target")
	if err != nil {
		return fmt.Errorf("failed to read --target flag: %w", err)
	}
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return fmt.Errorf("failed to read --dry-run flag: %w", err)
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return fmt.Errorf("failed to read --force flag: %w", err)
	}
	result, err := s.Sync(cmd.Context(), usecase.SyncOptions{Target: target, DryRun: dryRun, Force: force})
	if err != nil {
		return err
	}
	for _, targetResult := range result.Targets {
		if dryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "%s: %d written, %d would_write, %d skipped, %d failed\n",
				targetResult.Target,
				targetResult.FilesWritten,
				targetResult.FilesWouldWrite,
				targetResult.FilesSkipped,
				targetResult.FilesFailed,
			)
			for _, file := range targetResult.Files {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s %s\n", file.Status, file.Path)
			}
			continue
		}
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
}

func runAddSkill(cmd *cobra.Command, s service.Service, args []string) error {
	sourcePath := ""
	if len(args) > 1 {
		sourcePath = args[1]
	}
	if err := s.AddSkill(cmd.Context(), args[0], sourcePath); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Registered skill %s\n", args[0])
	return nil
}

func runRemoveSkill(cmd *cobra.Command, s service.Service, args []string) error {
	if err := s.RemoveSkill(cmd.Context(), args[0]); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed skill %s\n", args[0])
	return nil
}

func runListSkills(cmd *cobra.Command, s service.Service, _ []string) error {
	skills, err := s.ListSkills(cmd.Context())
	if err != nil {
		return err
	}
	for _, skill := range skills {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", skill.Name, skill.Path)
	}
	return nil
}

func runListTargets(cmd *cobra.Command, s service.Service, _ []string) error {
	targets, err := s.ListTargets(cmd.Context())
	if err != nil {
		return err
	}
	for _, target := range targets {
		status := "disabled"
		if target.Enabled {
			status = "enabled"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", target.Name, status, target.OutputDir, strings.Join(target.EmitPaths, ","))
	}
	return nil
}

func runEnableTarget(cmd *cobra.Command, s service.Service, args []string) error {
	if err := s.EnableTarget(cmd.Context(), args[0]); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Enabled target %s\n", args[0])
	return nil
}

func runDisableTarget(cmd *cobra.Command, s service.Service, args []string) error {
	if err := s.DisableTarget(cmd.Context(), args[0]); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Disabled target %s\n", args[0])
	return nil
}

func runPull(cmd *cobra.Command, s service.Service, args []string) error {
	remoteURL := ""
	if len(args) > 0 {
		remoteURL = args[0]
	}
	return s.Pull(cmd.Context(), remoteURL)
}

func runPush(cmd *cobra.Command, s service.Service, args []string) error {
	remoteURL := ""
	if len(args) > 0 {
		remoteURL = args[0]
	}
	return s.Push(cmd.Context(), remoteURL)
}
