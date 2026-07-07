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
		Use:   %[1]q,
		Short: %[3]q,
		Run: func(cmd *cobra.Command, args []string) {
			_ = s
		},
	}
	return cmd
}
`, name, method.Name, doc, quotedList(method.Params)))
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
