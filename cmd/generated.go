package cmd

import (
	"github.com/spf13/cobra"

	cmdgen "github.com/techgodhq/creed/cmd/gen"
	"github.com/techgodhq/creed/internal/service"
)

func registerGeneratedCommands(root *cobra.Command, svc service.Service) {
	commands := []*cobra.Command{
		cmdgen.NewInitCommand(svc),
		cmdgen.NewSyncCommand(svc),
		cmdgen.NewAddSkillCommand(svc),
		cmdgen.NewRemoveSkillCommand(svc),
		cmdgen.NewListSkillsCommand(svc),
		cmdgen.NewListTargetsCommand(svc),
		cmdgen.NewEnableTargetCommand(svc),
		cmdgen.NewDisableTargetCommand(svc),
		cmdgen.NewPullCommand(svc),
		cmdgen.NewPushCommand(svc),
	}
	for _, command := range commands {
		if commandNameExists(root, command.Name()) {
			// Preserve hand-written commands such as init and sync until their richer
			// UX is generated. Generated constructors are still built and conflict-
			// checked so adding them cannot create duplicate root commands.
			continue
		}
		root.AddCommand(command)
	}
}

func commandNameExists(root *cobra.Command, name string) bool {
	for _, command := range root.Commands() {
		if command.Name() == name {
			return true
		}
	}
	return false
}
