package gen

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	opsgen "github.com/techgodhq/creed/internal/ops/gen"
	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
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

// parseDebounce parses a human-friendly duration string like "500ms" or
// "1s" into a time.Duration. An empty string returns zero so the watch
// engine falls back to its default debounce window.
func parseDebounce(raw string) (time.Duration, error) {
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid --debounce %q: %w", raw, err)
	}
	return d, nil
}

// runWatchCommand turns the parsed CLI flags into a WatchOptions, wires
// up a stdout sink, installs signal handlers for Ctrl-C, and blocks on
// service.Service.Watch. It is the runtime companion to the generated
// runWatch handler; the split keeps imports for os/signal/syscall/time
// in the generated runtime, not in the generated handler file.
func runWatchCommand(cmd *cobra.Command, s service.Service, target string, quiet, force bool, debounceRaw string) error {
	debounce, err := parseDebounce(debounceRaw)
	if err != nil {
		return err
	}
	opts := usecase.WatchOptions{Target: target, Quiet: quiet, Force: force, Debounce: debounce}
	out := cmd.OutOrStdout()
	sink := func(summary usecase.WatchSummary) {
		if summary.Err != nil {
			fmt.Fprintf(out, "watch sync error: %v\n", summary.Err)
			return
		}
		if quiet || summary.Result == nil {
			return
		}
		for _, tr := range summary.Result.Targets {
			if tr.Error != nil {
				fmt.Fprintf(out, "[%s] %s: error: %v\n", summary.TriggeredAt.Format(time.RFC3339), tr.Target, tr.Error)
				continue
			}
			fmt.Fprintf(out, "[%s] %s: %d written, %d skipped, %d failed\n",
				summary.TriggeredAt.Format(time.RFC3339),
				tr.Target,
				tr.FilesWritten,
				tr.FilesSkipped,
				tr.FilesFailed,
			)
		}
	}
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(out, "watching .creed/ for changes (debounce %s, target %q)\n", usecase.EffectiveDebounce(debounce), target)
	if err := s.Watch(ctx, opts, sink); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}
