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
	"reflect"
	"strconv"
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
  --out-ops PATH    Output directory for generated operation descriptors (default: internal/ops/gen)
  --dry-run         Show what would be generated without writing files
  -h, --help        Show this help message

The generator reads the Service interface via Go AST, extracts method names,
parameter names, and doc comments, then produces thin wrappers that eliminate
drift between CLI, MCP, and future HTTP surfaces.
`

type serviceMethod struct {
	Name    string
	Doc     string
	Params  []methodParam
	Results []methodResult
}

type methodParam struct {
	Name         string
	ExternalName string
	Type         string
	Kind         string
	Required     bool
	CLIKind      string
	Help         string
}

type methodResult struct {
	Name string
	Type string
}

type structInfo struct {
	Name   string
	Fields []structField
}

type structField struct {
	Name     string
	JSONTag  string
	Embedded bool
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
		outOps      string
		dryRun      bool
		showHelp    bool
	)

	fs := flag.NewFlagSet("creed-codegen", flag.ContinueOnError)
	fs.StringVar(&servicePath, "service", "internal/service/service.go", "Path to the service interface Go file")
	fs.StringVar(&outCLI, "out-cli", "cmd/gen", "Output directory for generated CLI commands")
	fs.StringVar(&outMCP, "out-mcp", "internal/mcp/gen", "Output directory for generated MCP tools")
	fs.StringVar(&outOps, "out-ops", "internal/ops/gen", "Output directory for generated operation descriptors")
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
	if err := writeMCPHandlersFile(outMCP, methods, dryRun); err != nil {
		return err
	}
	if err := writeOperationDescriptorFile(outOps, methods, dryRun); err != nil {
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
	methods := collectInterfaceMethods(service, interfaces, map[string]bool{"Service": true})
	structs := collectStructsForService(path, file)
	if err := validateServiceMethods(methods, structs); err != nil {
		return nil, err
	}
	return methods, nil
}

func collectStructsForService(path string, file *ast.File) map[string]structInfo {
	structs := collectStructs(file, "")
	serviceDir := filepath.Dir(path)
	for _, parsed := range parsePackageFiles(serviceDir, filepath.Base(path)) {
		mergeStructs(structs, collectStructs(parsed, ""))
	}
	modulePath := modulePathFor(path)
	if modulePath == "" || file.Imports == nil {
		return structs
	}
	for _, importSpec := range file.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil || !strings.HasPrefix(importPath, modulePath+"/") {
			continue
		}
		pkgDir := filepath.Join(moduleRootFor(path), strings.TrimPrefix(importPath, modulePath+"/"))
		pkgName := packageNameForDir(pkgDir)
		if pkgName == "" {
			pkgName = filepath.Base(importPath)
		}
		if importSpec.Name != nil && importSpec.Name.Name != "_" && importSpec.Name.Name != "." {
			pkgName = importSpec.Name.Name
		}
		for _, parsed := range parsePackageFiles(pkgDir, "") {
			mergeStructs(structs, collectStructs(parsed, pkgName))
		}
	}
	return structs
}

func collectStructs(file *ast.File, qualifier string) map[string]structInfo {
	structs := map[string]structInfo{}
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
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			info := structInfo{Name: typeSpec.Name.Name}
			if qualifier != "" {
				info.Name = qualifier + "." + info.Name
			}
			if structType.Fields != nil {
				for _, field := range structType.Fields.List {
					jsonTag := ""
					if field.Tag != nil {
						if tag, err := strconv.Unquote(field.Tag.Value); err == nil {
							jsonTag = reflect.StructTag(tag).Get("json")
						}
					}
					if len(field.Names) == 0 {
						if name := embeddedFieldName(field.Type); name != "" {
							info.Fields = append(info.Fields, structField{Name: name, JSONTag: jsonTag, Embedded: true})
						}
						continue
					}
					for _, name := range field.Names {
						info.Fields = append(info.Fields, structField{Name: name.Name, JSONTag: jsonTag})
					}
				}
			}
			structs[info.Name] = info
		}
	}
	return structs
}

func parsePackageFiles(dir, skipBase string) []*ast.File {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	files := []*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == skipBase || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			continue
		}
		files = append(files, parsed)
	}
	return files
}

func packageNameForDir(dir string) string {
	for _, file := range parsePackageFiles(dir, "") {
		if file.Name != nil {
			return file.Name.Name
		}
	}
	return ""
}

func mergeStructs(dst, src map[string]structInfo) {
	for name, info := range src {
		dst[name] = info
	}
}

func modulePathFor(path string) string {
	root := moduleRootFor(path)
	if root == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func moduleRootFor(path string) string {
	dir := filepath.Dir(path)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func embeddedFieldName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	case *ast.StarExpr:
		return embeddedFieldName(typed.X)
	default:
		return ""
	}
}

func validateServiceMethods(methods []serviceMethod, structs map[string]structInfo) error {
	var problems []string
	for _, method := range methods {
		for _, param := range method.Params {
			switch param.Kind {
			case "context", "primitive":
				continue
			}
			if !isSupportedStructParam(param.Type, structs) {
				problems = append(problems, fmt.Sprintf("%s.%s has unsupported input type %s; supported inputs are context.Context, primitive params (string, bool, int, int64, float64), no-input methods, and struct Options/Request params with json tags", method.Name, param.Name, param.Type))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("service interface contains unsupported generated input shapes:\n- %s", strings.Join(problems, "\n- "))
	}
	return nil
}

func isSupportedStructParam(typeName string, structs map[string]structInfo) bool {
	if strings.HasPrefix(typeName, "[]") || strings.HasPrefix(typeName, "*") || strings.HasPrefix(typeName, "map[") || strings.HasPrefix(typeName, "chan ") || strings.Contains(typeName, " chan ") {
		return false
	}
	shortName := typeName
	if idx := strings.LastIndex(shortName, "."); idx >= 0 {
		shortName = shortName[idx+1:]
	}
	if !(strings.HasSuffix(shortName, "Options") || strings.HasSuffix(shortName, "Request")) {
		return false
	}
	info, ok := structs[typeName]
	if !ok {
		return false
	}
	for _, field := range info.Fields {
		if field.Embedded {
			return false
		}
		if !ast.IsExported(field.Name) {
			continue
		}
		if field.JSONTag == "" || field.JSONTag == "-" {
			return false
		}
	}
	return true
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
				Name:    name.Name,
				Doc:     strings.TrimSpace(field.Doc.Text()),
				Params:  methodParams(fn),
				Results: methodResults(fn),
			})
		}
	}
	return methods
}

func methodParams(fn *ast.FuncType) []methodParam {
	if fn == nil || fn.Params == nil {
		return nil
	}
	params := []methodParam{}
	unnamed := 1
	for _, field := range fn.Params.List {
		typeName := exprString(field.Type)
		if len(field.Names) == 0 {
			name := fmt.Sprintf("param%d", unnamed)
			params = append(params, methodParam{Name: name, ExternalName: externalName(name), Type: typeName, Kind: inputKind(typeName)})
			unnamed++
			continue
		}
		for _, name := range field.Names {
			params = append(params, methodParam{Name: name.Name, ExternalName: externalName(name.Name), Type: typeName, Kind: inputKind(typeName)})
		}
	}
	return params
}

func methodResults(fn *ast.FuncType) []methodResult {
	if fn == nil || fn.Results == nil {
		return nil
	}
	results := []methodResult{}
	unnamed := 1
	for _, field := range fn.Results.List {
		typeName := exprString(field.Type)
		if len(field.Names) == 0 {
			results = append(results, methodResult{Name: fmt.Sprintf("result%d", unnamed), Type: typeName})
			unnamed++
			continue
		}
		for _, name := range field.Names {
			results = append(results, methodResult{Name: name.Name, Type: typeName})
		}
	}
	return results
}

func exprString(expr ast.Expr) string {
	var b strings.Builder
	if err := format.Node(&b, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return b.String()
}

func inputKind(typeName string) string {
	switch typeName {
	case "context.Context":
		return "context"
	case "string", "bool", "int", "int64", "float64":
		return "primitive"
	default:
		return "struct"
	}
}

func externalName(name string) string {
	if name == "ctx" {
		return "context"
	}
	return snakeCase(name)
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
	content, err := formatGo(cliRuntimeSource)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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

func writeMCPHandlersFile(dir string, methods []serviceMethod, dryRun bool) error {
	path := filepath.Join(dir, "handlers.go")
	if dryRun {
		fmt.Printf("would write %s\n", path)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", dir, err)
	}
	content, err := mcpHandlersContent(methods)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func writeOperationDescriptorFile(dir string, methods []serviceMethod, dryRun bool) error {
	path := filepath.Join(dir, "operations.go")
	if dryRun {
		fmt.Printf("would write %s\n", path)
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output dir %s: %w", dir, err)
	}
	content, err := operationDescriptorContent(methods)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func operationDescriptorContent(methods []serviceMethod) (string, error) {
	var ops strings.Builder
	for _, method := range methods {
		doc := method.Doc
		if doc == "" {
			doc = method.Name + " invokes service.Service." + method.Name
		}
		operationName := snakeCase(method.Name)
		fmt.Fprintf(&ops, "	{\n")
		fmt.Fprintf(&ops, "		MethodName: %s,\n", strconv.Quote(method.Name))
		fmt.Fprintf(&ops, "		OperationName: %s,\n", strconv.Quote(operationName))
		fmt.Fprintf(&ops, "		Description: %s,\n", strconv.Quote(doc))
		fmt.Fprintf(&ops, "		CLIName: %s,\n", strconv.Quote(cliCommandName(method.Name, operationName)))
		fmt.Fprintf(&ops, "		MCPName: %s,\n", strconv.Quote(operationName))
		fmt.Fprintf(&ops, "		HTTPRoute: %s,\n", strconv.Quote("/v1/operations/"+operationName))
		fmt.Fprintf(&ops, "		Inputs: []InputDescriptor{%s},\n", inputDescriptors(operationInputs(method)))
		fmt.Fprintf(&ops, "		Outputs: []OutputDescriptor{%s},\n", outputDescriptors(method.Results))
		fmt.Fprintf(&ops, "	},\n")
	}

	return formatGo(fmt.Sprintf(`// Code generated by creed-codegen; DO NOT EDIT.

// Package gen contains generated operation descriptors derived from service.Service.
package gen

// OperationDescriptor describes one service.Service operation for generated surfaces.
type OperationDescriptor struct {
	MethodName    string
	OperationName string
	Description   string
	CLIName       string
	MCPName       string
	HTTPRoute     string
	Inputs        []InputDescriptor
	Outputs       []OutputDescriptor
}

// InputDescriptor describes one generated operation input.
type InputDescriptor struct {
	Name         string
	ExternalName string
	Type         string
	Kind         string
	Required     bool
	CLIKind      string
	Help         string
}

// OutputDescriptor describes one generated operation output.
type OutputDescriptor struct {
	Name string
	Type string
}

// Operations contains one descriptor per service.Service method.
var Operations = []OperationDescriptor{
%s}

// ByOperationName returns the descriptor for operationName, if generated.
func ByOperationName(operationName string) (OperationDescriptor, bool) {
	for _, operation := range Operations {
		if operation.OperationName == operationName {
			return operation, true
		}
	}
	return OperationDescriptor{}, false
}
`, ops.String()))
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

func mcpHandlersContent(methods []serviceMethod) (string, error) {
	var tools strings.Builder
	var handlers strings.Builder
	for _, method := range methods {
		inputs := operationInputs(method)
		fmt.Fprintf(&tools, "\t{Spec: %sToolSpec(), Tool: %sMCPTool(), Handler: %sMCPHandler(s)},\n", method.Name, method.Name, method.Name)
		handler, err := mcpHandlerFunction(method, inputs)
		if err != nil {
			return "", err
		}
		handlers.WriteString(handler)
	}
	return formatGo(fmt.Sprintf(`// Code generated by creed-codegen; DO NOT EDIT.

package gen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
)

// ToolHandler invokes one generated MCP tool with decoded JSON input.
type ToolHandler func(context.Context, json.RawMessage) (any, error)

// GeneratedTool contains a fully generated MCP tool definition and handler.
type GeneratedTool struct {
	Spec    ToolSpec
	Tool    mcplib.Tool
	Handler ToolHandler
}

// GeneratedTools returns all Service-derived MCP tools and handlers.
func GeneratedTools(s service.Service) []GeneratedTool {
	return []GeneratedTool{
%s	}
}

%s
type okResponse struct {
	OK bool `+"`json:\"ok\"`"+`
}

func decodePayload(payload json.RawMessage, dst any) error {
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode MCP tool payload: %%w", err)
	}
	return nil
}
`, tools.String(), handlers.String()))
}

func mcpHandlerFunction(method serviceMethod, inputs []methodParam) (string, error) {
	requestType := lowerFirst(method.Name) + "Request"
	var b strings.Builder
	if len(inputs) > 0 {
		fmt.Fprintf(&b, "type %s struct {\n", requestType)
		for _, input := range inputs {
			fieldName := exportedName(input.Name)
			jsonTag := input.ExternalName
			if !input.Required {
				jsonTag += ",omitempty"
			}
			fmt.Fprintf(&b, "\t%s %s `json:%q`\n", fieldName, input.Type, jsonTag)
		}
		fmt.Fprintf(&b, "}\n\n")
	}
	fmt.Fprintf(&b, "// %sToolSpec returns generated MCP metadata for service.Service.%s.\n", method.Name, method.Name)
	fmt.Fprintf(&b, "func %sToolSpec() ToolSpec {\n", method.Name)
	fmt.Fprintf(&b, "	return ToolSpec{MethodName: %q, Name: %sToolName, Description: %sToolDescription, ParamNames: []string{%s}}\n", method.Name, method.Name, method.Name, quotedList(externalInputNames(inputs)))
	fmt.Fprintf(&b, "}\n\n")
	fmt.Fprintf(&b, "// %sMCPTool returns the generated MCP tool definition for service.Service.%s.\n", method.Name, method.Name)
	fmt.Fprintf(&b, "func %sMCPTool() mcplib.Tool {\n", method.Name)
	fmt.Fprintf(&b, "\toptions := []mcplib.ToolOption{mcplib.WithDescription(%sToolDescription)}\n", method.Name)
	for _, input := range inputs {
		fmt.Fprintf(&b, "\toptions = append(options, %s)\n", mcpToolOption(input))
	}
	fmt.Fprintf(&b, "	return mcplib.NewTool(%sToolName, options...)\n", method.Name)
	fmt.Fprintf(&b, "}\n\n")
	fmt.Fprintf(&b, "// %sMCPHandler returns the generated MCP handler for service.Service.%s.\n", method.Name, method.Name)
	fmt.Fprintf(&b, "func %sMCPHandler(s service.Service) ToolHandler {\n", method.Name)
	fmt.Fprintf(&b, "\treturn func(ctx context.Context, payload json.RawMessage) (any, error) {\n")
	if len(inputs) > 0 {
		fmt.Fprintf(&b, "\t\tvar req %s\n", requestType)
		fmt.Fprintf(&b, "\t\tif err := decodePayload(payload, &req); err != nil {\n\t\t\treturn nil, err\n\t\t}\n")
		for _, input := range inputs {
			if input.Required && input.Type == "string" {
				fmt.Fprintf(&b, "		if strings.TrimSpace(req.%s) == \"\" {\n			return nil, fmt.Errorf(%q)\n		}\n", exportedName(input.Name), input.ExternalName+" is required")
			}
		}
	} else {
		fmt.Fprintf(&b, "\t\tif err := decodePayload(payload, &struct{}{}); err != nil {\n\t\t\treturn nil, err\n\t\t}\n")
	}
	callArgs, err := mcpCallArgs(method, inputs)
	if err != nil {
		return "", err
	}
	if len(method.Results) == 1 && method.Results[0].Type == "error" {
		fmt.Fprintf(&b, "\t\tif err := s.%s(%s); err != nil {\n\t\t\treturn nil, err\n\t\t}\n", method.Name, callArgs)
		fmt.Fprintf(&b, "\t\treturn okResponse{OK: true}, nil\n")
	} else if len(method.Results) == 2 && method.Results[1].Type == "error" {
		fmt.Fprintf(&b, "\t\tresult, err := s.%s(%s)\n", method.Name, callArgs)
		fmt.Fprintf(&b, "\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n")
		fmt.Fprintf(&b, "\t\treturn result, nil\n")
	} else {
		return "", fmt.Errorf("cannot generate MCP handler for %s: unsupported result shape", method.Name)
	}
	fmt.Fprintf(&b, "\t}\n")
	fmt.Fprintf(&b, "}\n\n")
	return b.String(), nil
}

func mcpCallArgs(method serviceMethod, inputs []methodParam) (string, error) {
	args := []string{}
	inputByName := map[string]methodParam{}
	for _, input := range inputs {
		inputByName[input.Name] = input
	}
	for _, param := range method.Params {
		switch param.Kind {
		case "context":
			args = append(args, "ctx")
		case "primitive":
			input, ok := inputByName[param.Name]
			if !ok {
				input = param
			}
			args = append(args, "req."+exportedName(input.Name))
		case "struct":
			for _, input := range inputs {
				if input.Kind == "struct" {
					return "", fmt.Errorf("cannot generate MCP handler for %s.%s: struct inputs must be expanded into operation descriptor fields", method.Name, param.Name)
				}
			}
			parts := []string{}
			for _, input := range inputs {
				parts = append(parts, fmt.Sprintf("%s: req.%s", exportedName(input.Name), exportedName(input.Name)))
			}
			args = append(args, fmt.Sprintf("%s{%s}", param.Type, strings.Join(parts, ", ")))
		default:
			return "", fmt.Errorf("cannot generate MCP call for %s.%s kind %s", method.Name, param.Name, param.Kind)
		}
	}
	return strings.Join(args, ", "), nil
}

func mcpToolOption(input methodParam) string {
	var option string
	switch input.Type {
	case "bool":
		option = fmt.Sprintf("mcplib.WithBoolean(%q", input.ExternalName)
	case "int", "int64":
		option = fmt.Sprintf("mcplib.WithInteger(%q", input.ExternalName)
	case "float64":
		option = fmt.Sprintf("mcplib.WithNumber(%q", input.ExternalName)
	default:
		option = fmt.Sprintf("mcplib.WithString(%q", input.ExternalName)
	}
	if input.Required {
		option += ", mcplib.Required()"
	}
	return option + ")"
}

func exportedName(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func lowerFirst(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(name[:1]) + name[1:]
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
`, name, method.Name, doc, quotedList(paramNamesFrom(method.Params)), cliUse(method.Name, name), cliArgs(method.Name), cliRunner(method.Name), cliExtra(method.Name)))
	case "MCP":
		return formatGo(fmt.Sprintf(`// Code generated by creed-codegen; DO NOT EDIT.

package gen

// %[2]sToolName is the generated MCP tool name for service.Service.%[2]s.
const %[2]sToolName = %[1]q

// %[2]sToolDescription is the generated MCP tool description for service.Service.%[2]s.
const %[2]sToolDescription = %[3]q

// %[2]sToolParams are parameter names extracted from service.Service.%[2]s.
var %[2]sToolParams = []string{%[4]s}
`, name, method.Name, doc, quotedList(paramNamesFrom(method.Params))))
	default:
		return "// Code generated by creed-codegen; DO NOT EDIT.\n\npackage gen\n", nil
	}
}

func formatGo(src string) (string, error) {
	formatted, err := format.Source([]byte(src))
	if err != nil {
		return "", fmt.Errorf("format generated source: %w", err)
	}
	return groupLocalImports(string(formatted)), nil
}

func groupLocalImports(src string) string {
	return strings.ReplaceAll(
		src,
		"\"github.com/spf13/cobra\"\n	\"github.com/techgodhq/creed/",
		"\"github.com/spf13/cobra\"\n\n	\"github.com/techgodhq/creed/",
	)
}

func quotedList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}
	return strings.Join(quoted, ", ")
}

func externalInputNames(inputs []methodParam) []string {
	names := make([]string, 0, len(inputs))
	for _, input := range inputs {
		names = append(names, input.ExternalName)
	}
	return names
}

func paramNamesFrom(params []methodParam) []string {
	names := make([]string, 0, len(params))
	for _, param := range params {
		names = append(names, param.Name)
	}
	return names
}

func inputDescriptors(params []methodParam) string {
	parts := make([]string, 0, len(params))
	for _, param := range params {
		parts = append(parts, fmt.Sprintf("{Name: %s, ExternalName: %s, Type: %s, Kind: %s, Required: %t, CLIKind: %s, Help: %s}",
			strconv.Quote(param.Name),
			strconv.Quote(param.ExternalName),
			strconv.Quote(param.Type),
			strconv.Quote(param.Kind),
			param.Required,
			strconv.Quote(param.CLIKind),
			strconv.Quote(param.Help),
		))
	}
	return strings.Join(parts, ", ")
}

func operationInputs(method serviceMethod) []methodParam {
	switch method.Name {
	case "Init":
		return []methodParam{{Name: "projectName", ExternalName: "project_name", Type: "string", Kind: "primitive", Required: true, CLIKind: "arg", Help: "Project name for the generated scaffold."}}
	case "Sync":
		return []methodParam{
			{Name: "target", ExternalName: "target", Type: "string", Kind: "primitive", CLIKind: "flag", Help: "Emit for a specific target (claude, cursor, codex, windsurf, aider)."},
			{Name: "dryRun", ExternalName: "dry_run", Type: "bool", Kind: "primitive", CLIKind: "flag", Help: "Show files that would be emitted without writing."},
			{Name: "force", ExternalName: "force", Type: "bool", Kind: "primitive", CLIKind: "flag", Help: "Rewrite files even when content is unchanged."},
		}
	case "AddSkill":
		return []methodParam{
			{Name: "name", ExternalName: "name", Type: "string", Kind: "primitive", Required: true, CLIKind: "arg", Help: "Skill name."},
			{Name: "sourcePath", ExternalName: "source_path", Type: "string", Kind: "primitive", CLIKind: "arg", Help: "Optional source skill file path."},
		}
	case "RemoveSkill", "EnableTarget", "DisableTarget":
		return []methodParam{{Name: "name", ExternalName: "name", Type: "string", Kind: "primitive", Required: true, CLIKind: "arg", Help: "Target or skill name."}}
	case "Pull", "Push":
		return []methodParam{{Name: "remoteURL", ExternalName: "remote_url", Type: "string", Kind: "primitive", CLIKind: "arg", Help: "Optional git remote URL override."}}
	default:
		return nonContextParams(method.Params)
	}
}

func nonContextParams(params []methodParam) []methodParam {
	filtered := make([]methodParam, 0, len(params))
	for _, param := range params {
		if param.Kind == "context" || param.Type == "context.Context" {
			continue
		}
		filtered = append(filtered, param)
	}
	return filtered
}

func outputDescriptors(results []methodResult) string {
	parts := make([]string, 0, len(results))
	for _, result := range results {
		parts = append(parts, fmt.Sprintf("{Name: %s, Type: %s}", strconv.Quote(result.Name), strconv.Quote(result.Type)))
	}
	return strings.Join(parts, ", ")
}

func cliCommandName(methodName, fallback string) string {
	return strings.Fields(cliUse(methodName, fallback))[0]
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
	runes := []rune(name)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || nextLower {
				out.WriteByte('_')
			}
		}
		out.WriteRune(unicode.ToLower(r))
	}
	return out.String()
}
