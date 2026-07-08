// Package codegen implements the code generation tool that produces thin
// wrappers (CLI commands, MCP tools) from the Service interface.
//
// This entrypoint reads the canonical Service interface and emits generated
// surface scaffolding so CLI, MCP, and future wrappers do not drift.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"
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

The generator reads the Service interface via Go AST, extracts method names,
parameter names, and doc comments, then produces thin wrappers that eliminate
drift between CLI, MCP, and future HTTP surfaces.
`

type serviceMethod struct {
	Name   string
	Doc    string
	Params []string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var (
		servicePath string
		outCLI      string
		outMCP      string
		dryRun      bool
		showHelp    bool
	)

	fs := flag.NewFlagSet("creed-codegen", flag.ContinueOnError)
	fs.StringVar(&servicePath, "service", "internal/service/service.go", "Path to the service interface Go file")
	fs.StringVar(&outCLI, "out-cli", "cmd/gen", "Output directory for generated CLI commands")
	fs.StringVar(&outMCP, "out-mcp", "internal/mcp/gen", "Output directory for generated MCP tools")
	fs.BoolVar(&dryRun, "dry-run", false, "Show what would be generated without writing files")
	fs.BoolVar(&showHelp, "help", false, "Show help message")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, helpText)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if showHelp {
		fmt.Print(helpText)
		return nil
	}

	methods, err := serviceMethods(servicePath)
	if err != nil {
		return err
	}
	for _, method := range methods {
		name := snakeCase(method.Name)
		if err := writeGeneratedFile(outCLI, name, method, "CLI", dryRun); err != nil {
			return err
		}
		if err := writeGeneratedFile(outMCP, name, method, "MCP", dryRun); err != nil {
			return err
		}
	}
	if err := writeCLIRuntimeFile(outCLI, dryRun); err != nil {
		return err
	}
	if err := writeMCPRegistryFile(outMCP, methods, dryRun); err != nil {
		return err
	}
	return nil
}

func serviceMethods(path string) ([]serviceMethod, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse service file: %w", err)
	}
	interfaces := map[string]*ast.InterfaceType{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			iface, ok := typeSpec.Type.(*ast.InterfaceType)
			if ok {
				interfaces[typeSpec.Name.Name] = iface
			}
		}
	}
	service, ok := interfaces["Service"]
	if !ok {
		return nil, fmt.Errorf("Service interface not found in %s", path)
	}
	return collectInterfaceMethods(service, interfaces, map[string]bool{"Service": true}), nil
}

func collectInterfaceMethods(iface *ast.InterfaceType, interfaces map[string]*ast.InterfaceType, seen map[string]bool) []serviceMethod {
	methods := make([]serviceMethod, 0, len(iface.Methods.List))
	for _, field := range iface.Methods.List {
		if len(field.Names) == 0 {
			ident, ok := field.Type.(*ast.Ident)
			if !ok || interfaces[ident.Name] == nil || seen[ident.Name] {
				continue
			}
			seen[ident.Name] = true
			methods = append(methods, collectInterfaceMethods(interfaces[ident.Name], interfaces, seen)...)
			continue
		}

		fn, _ := field.Type.(*ast.FuncType)
		for _, name := range field.Names {
			methods = append(methods, serviceMethod{
				Name:   name.Name,
				Doc:    strings.TrimSpace(field.Doc.Text()),
				Params: paramNames(fn),
			})
		}
	}
	return methods
}

func paramNames(fn *ast.FuncType) []string {
	if fn == nil || fn.Params == nil {
		return nil
	}
	params := []string{}
	unnamed := 1
	for _, field := range fn.Params.List {
		if len(field.Names) == 0 {
			params = append(params, fmt.Sprintf("param%d", unnamed))
			unnamed++
			continue
		}
		for _, name := range field.Names {
			params = append(params, name.Name)
		}
	}
	return params
}

func writeCLIRuntimeFile(dir string, dryRun bool) error {
	path := filepath.Join(dir, "runtime.go")
	if dryRun {
		fmt.Printf("would write %s\n", path)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(cliRuntimeSource), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeGeneratedFile(dir, name string, method serviceMethod, surface string, dryRun bool) error {
	path := filepath.Join(dir, name+".go")
	if dryRun {
		fmt.Printf("would write %s\n", path)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", dir, err)
	}
	content, err := generatedContent(name, method, surface)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeMCPRegistryFile(dir string, methods []serviceMethod, dryRun bool) error {
	path := filepath.Join(dir, "tool_specs.go")
	if dryRun {
		fmt.Printf("would write %s\n", path)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", dir, err)
	}
	content, err := mcpRegistryContent(methods)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func mcpRegistryContent(methods []serviceMethod) (string, error) {
	var specs strings.Builder
	for _, method := range methods {
		fmt.Fprintf(&specs, "	{MethodName: %q, Name: %sToolName, Description: %sToolDescription, ParamNames: %sToolParams},\n", method.Name, method.Name, method.Name, method.Name)
	}
	return formatGo(fmt.Sprintf(`// Code generated by creed-codegen; DO NOT EDIT.

package gen

// ToolSpec describes a generated MCP tool derived from service.Service.
type ToolSpec struct {
	MethodName  string
	Name        string
	Description string
	ParamNames  []string
}

// ToolSpecs contains one generated MCP tool spec per service.Service method.
var ToolSpecs = []ToolSpec{
%s}
`, specs.String()))
}

func generatedContent(name string, method serviceMethod, surface string) (string, error) {
	doc := method.Doc
	if doc == "" {
		doc = method.Name + " invokes service.Service." + method.Name
	}
	switch surface {
	case "CLI":
		return formatGo(fmt.Sprintf(`// Code generated by creed-codegen; DO NOT EDIT.

package gen

import (
	"github.com/spf13/cobra"
	"github.com/techgodhq/creed/internal/service"
)

// %[2]sCommandSpec describes the generated CLI wrapper for service.Service.%[2]s.
type %[2]sCommandSpec struct {
	MethodName string
	ParamNames []string
}

// %[2]sSpec is metadata extracted from service.Service.%[2]s.
var %[2]sSpec = %[2]sCommandSpec{
	MethodName: %[2]q,
	ParamNames: []string{%[4]s},
}

// New%[2]sCommand returns the generated Cobra command wrapper for service.Service.%[2]s.
func New%[2]sCommand(s service.Service) *cobra.Command {
	cmd := &cobra.Command{
		Use:   %[5]q,
		Short: %[3]q,
		Args:  %[6]s,
		RunE: func(cmd *cobra.Command, args []string) error {
			return %[7]s(cmd, s, args)
		},
	}
%[8]s	return cmd
}
`, name, method.Name, doc, quotedList(method.Params), cliUse(method.Name, name), cliArgs(method.Name), cliRunner(method.Name), cliExtra(method.Name)))
	case "MCP":
		return formatGo(fmt.Sprintf(`// Code generated by creed-codegen; DO NOT EDIT.

package gen

// %[2]sToolName is the generated MCP tool name for service.Service.%[2]s.
const %[2]sToolName = %[1]q

// %[2]sToolDescription is the generated MCP tool description for service.Service.%[2]s.
const %[2]sToolDescription = %[3]q

// %[2]sToolParams are parameter names extracted from service.Service.%[2]s.
var %[2]sToolParams = []string{%[4]s}
`, name, method.Name, doc, quotedList(method.Params)))
	default:
		return "// Code generated by creed-codegen; DO NOT EDIT.\n\npackage gen\n", nil
	}
}

func formatGo(src string) (string, error) {
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return "", fmt.Errorf("format generated source: %w", err)
	}
	return string(formatted), nil
}

func quotedList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ", ")
}

func cliUse(methodName, fallback string) string {
	switch methodName {
	case "Init":
		return "init [project-name]"
	case "AddSkill":
		return "add-skill <name> [source-path]"
	case "RemoveSkill":
		return "remove-skill <name>"
	case "ListSkills":
		return "list-skills"
	case "ListTargets":
		return "list-targets"
	case "EnableTarget":
		return "enable-target <name>"
	case "DisableTarget":
		return "disable-target <name>"
	case "Pull":
		return "pull [remote-url]"
	case "Push":
		return "push [remote-url]"
	case "Sync":
		return "sync"
	default:
		return fallback
	}
}

func cliArgs(methodName string) string {
	switch methodName {
	case "AddSkill":
		return "cobra.RangeArgs(1, 2)"
	case "Init", "Pull", "Push":
		return "cobra.MaximumNArgs(1)"
	case "RemoveSkill", "EnableTarget", "DisableTarget":
		return "cobra.ExactArgs(1)"
	default:
		return "cobra.NoArgs"
	}
}

func cliRunner(methodName string) string {
	return "run" + methodName
}

func cliExtra(methodName string) string {
	if methodName != "Sync" {
		return ""
	}
	return "	cmd.Flags().StringP(\"target\", \"t\", \"\", \"emit for a specific target (claude, cursor, codex, windsurf, aider)\")\n	cmd.Flags().Bool(\"dry-run\", false, \"show files that would be emitted without writing\")\n	cmd.Flags().Bool(\"force\", false, \"rewrite files even when content is unchanged\")\n"
}

const cliRuntimeSource = `package gen

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
`

func snakeCase(name string) string {
	var out strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			out.WriteByte('_')
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}
