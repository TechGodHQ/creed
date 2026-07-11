package gen

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	opsgen "github.com/techgodhq/creed/internal/ops/gen"
	"github.com/techgodhq/creed/internal/service"
)

type commandRunner func(*cobra.Command, service.Service, []string) error

func mustOperation(methodName string) opsgen.OperationDescriptor {
	operation, ok := opsgen.ByMethodName(methodName)
	if !ok {
		panic(fmt.Sprintf("generated CLI operation %s missing descriptor", methodName))
	}
	return operation
}

func newGeneratedCommand(s service.Service, operation opsgen.OperationDescriptor, runner commandRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   cliUse(operation),
		Short: operation.Description,
		Args:  cliArgs(operation),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, s, args)
		},
	}
	for _, input := range operation.Inputs {
		if input.CLIKind != "flag" {
			continue
		}
		flagName := cliFlagName(input.ExternalName)
		help := input.Help
		switch input.Type {
		case "bool":
			cmd.Flags().Bool(flagName, false, help)
		case "string":
			if input.Name == "target" {
				cmd.Flags().StringP(flagName, "t", "", help)
			} else {
				cmd.Flags().String(flagName, "", help)
			}
		}
	}
	return cmd
}

func cliUse(operation opsgen.OperationDescriptor) string {
	parts := []string{operation.CLIName}
	for _, input := range operation.Inputs {
		if input.CLIKind != "arg" {
			continue
		}
		name := strings.ReplaceAll(input.ExternalName, "_", "-")
		if input.Required && operation.MethodName != "Init" {
			parts = append(parts, "<"+name+">")
		} else {
			parts = append(parts, "["+name+"]")
		}
	}
	return strings.Join(parts, " ")
}

func cliArgs(operation opsgen.OperationDescriptor) cobra.PositionalArgs {
	minArgs := 0
	maxArgs := 0
	for _, input := range operation.Inputs {
		if input.CLIKind != "arg" {
			continue
		}
		maxArgs++
		if input.Required && operation.MethodName != "Init" {
			minArgs++
		}
	}
	return cobra.RangeArgs(minArgs, maxArgs)
}

func cliFlagName(externalName string) string {
	return strings.ReplaceAll(externalName, "_", "-")
}

func positionalInput(args []string, index int) string {
	if index >= len(args) {
		return ""
	}
	return args[index]
}

func stringFlag(cmd *cobra.Command, externalName string) (string, error) {
	flagName := cliFlagName(externalName)
	value, err := cmd.Flags().GetString(flagName)
	if err != nil {
		return "", fmt.Errorf("failed to read --%s flag: %w", flagName, err)
	}
	return value, nil
}

func boolFlag(cmd *cobra.Command, externalName string) (bool, error) {
	flagName := cliFlagName(externalName)
	value, err := cmd.Flags().GetBool(flagName)
	if err != nil {
		return false, fmt.Errorf("failed to read --%s flag: %w", flagName, err)
	}
	return value, nil
}
