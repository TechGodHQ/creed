package cmd

import (
	"github.com/spf13/cobra"

	cmdgen "github.com/techgodhq/creed/cmd/gen"
	"github.com/techgodhq/creed/internal/service"
)

func registerGeneratedCommands(root *cobra.Command, svc service.Service) {
	for _, command := range cmdgen.Commands(svc) {
		root.AddCommand(command)
	}
}
