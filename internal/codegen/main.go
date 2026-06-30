// Package codegen implements the code generation tool that produces thin
// wrappers (CLI commands, MCP tools) from the Service interface.
//
// This is the scaffolding entrypoint — the full reflection-based generator
// is implemented in later PRs (T19). For now this provides the CLI
// structure and help text so the build system can wire go:generate.
package main

import (
	"flag"
	"fmt"
	"os"
)

const helpText = `creed-codegen generates CLI and MCP surface code from the Service interface.

Usage:
  creed-codegen [flags]

Flags:
  --service PATH    Path to the service interface Go file (default: internal/service/service.go)
  --out-cli PATH    Output directory for generated CLI commands (default: cmd/gen)
  --out-mcp PATH    Output directory for generated MCP tools (default: internal/mcp/gen)
  --dry-run         Show what would be generated without writing files
  -h, --help        Show this help message

The generator reads the Service interface via Go reflection, extracts method
names, parameter types, and doc comments, then produces thin wrappers that
eliminate drift between CLI, MCP, and future HTTP surfaces.
`

func main() {
	var (
		servicePath string
		outCLI      string
		outMCP      string
		dryRun      bool
		showHelp    bool
	)

	flag.StringVar(&servicePath, "service", "internal/service/service.go", "Path to the service interface Go file")
	flag.StringVar(&outCLI, "out-cli", "cmd/gen", "Output directory for generated CLI commands")
	flag.StringVar(&outMCP, "out-mcp", "internal/mcp/gen", "Output directory for generated MCP tools")
	flag.BoolVar(&dryRun, "dry-run", false, "Show what would be generated without writing files")
	flag.BoolVar(&showHelp, "help", false, "Show help message")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, helpText)
	}

	flag.Parse()

	if showHelp {
		fmt.Print(helpText)
		os.Exit(0)
	}

	// Scaffolding: the full generator is implemented in T19.
	// For now, just validate that the service file exists.
	if _, err := os.Stat(servicePath); err != nil {
		fmt.Fprintf(os.Stderr, "creed-codegen: service file not found: %s\n", servicePath)
		fmt.Fprintln(os.Stderr, "Note: code generation requires the Service interface to exist (T16+).")
		os.Exit(1)
	}

	fmt.Printf("creed-codegen: scaffolding mode (full generator pending T19)\n")
	fmt.Printf("  service: %s\n", servicePath)
	fmt.Printf("  cli out: %s\n", outCLI)
	fmt.Printf("  mcp out: %s\n", outMCP)
	if dryRun {
		fmt.Println("  mode:    dry-run")
	}

	// TODO(T19): implement reflection-based code generation.
}
