package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesCLIAndMCPFilesForServiceMethods(t *testing.T) {
	root := repoRoot(t)
	outCLI := filepath.Join(t.TempDir(), "cmd", "gen")
	outMCP := filepath.Join(t.TempDir(), "internal", "mcp", "gen")
	outOps := filepath.Join(t.TempDir(), "internal", "ops", "gen")
	outHTTP := filepath.Join(t.TempDir(), "internal", "httpapi", "gen")

	err := run([]string{
		"--service", filepath.Join(root, "internal", "service", "service.go"),
		"--out-cli", outCLI,
		"--out-mcp", outMCP,
		"--out-ops", outOps,
		"--out-http", outHTTP,
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	for _, method := range []string{
		"init",
		"sync",
		"add_skill",
		"remove_skill",
		"list_skills",
		"list_targets",
		"enable_target",
		"disable_target",
		"pull",
		"push",
	} {
		assertFileExists(t, filepath.Join(outCLI, method+".go"))
		assertFileExists(t, filepath.Join(outMCP, method+".go"))
	}
	assertFileExists(t, filepath.Join(outHTTP, "handlers.go"))
	cliSync, err := os.ReadFile(filepath.Join(outCLI, "sync.go"))
	if err != nil {
		t.Fatalf("read generated CLI sync file: %v", err)
	}
	if !strings.Contains(string(cliSync), "func NewSyncCommand(s service.Service) *cobra.Command") {
		t.Fatalf("generated CLI sync file does not expose NewSyncCommand wrapper:\n%s", string(cliSync))
	}
	if !strings.Contains(string(cliSync), `Operation:  mustOperation("Sync")`) {
		t.Fatalf("generated CLI sync file does not consume the operation descriptor:\n%s", string(cliSync))
	}
	if !strings.Contains(string(cliSync), `return newGeneratedCommand(s, SyncSpec.Operation, runSync)`) {
		t.Fatalf("generated CLI sync file does not delegate through descriptor-driven runtime:\n%s", string(cliSync))
	}
	if !strings.Contains(string(cliSync), `ParamNames: []string{"ctx", "opts"}`) {
		t.Fatalf("generated CLI sync file does not expose parameter metadata:\n%s", string(cliSync))
	}
	if strings.Contains(string(cliSync), "not wired yet") {
		t.Fatalf("generated CLI sync file should not emit unwired runtime errors:\n%s", string(cliSync))
	}

	cliRuntime, err := os.ReadFile(filepath.Join(outCLI, "runtime.go"))
	if err != nil {
		t.Fatalf("read generated CLI runtime file: %v", err)
	}
	cliRuntimeContent := string(cliRuntime)
	for _, want := range []string{
		`func newGeneratedCommand(s service.Service, operation opsgen.OperationDescriptor, runner commandRunner) *cobra.Command`,
		`Use:   cliUse(operation)`,
		`Short: operation.Description`,
		`cmd.Flags().StringP(flagName, "t", "", help)`,
	} {
		if !strings.Contains(cliRuntimeContent, want) {
			t.Fatalf("generated CLI runtime missing %q:\n%s", want, cliRuntimeContent)
		}
	}

	cliHandlers, err := os.ReadFile(filepath.Join(outCLI, "handlers.go"))
	if err != nil {
		t.Fatalf("read generated CLI handlers file: %v", err)
	}
	cliHandlersContent := string(cliHandlers)
	for _, want := range []string{
		`func runSync(cmd *cobra.Command, s service.Service, args []string) error`,
		`target, err := stringFlag(cmd, "target")`,
		`dryRun, err := boolFlag(cmd, "dry_run")`,
		`result, err := s.Sync(cmd.Context(), usecase.SyncOptions{Target: target, DryRun: dryRun, Force: force})`,
		`func runAddSkill(cmd *cobra.Command, s service.Service, args []string) error`,
		`if err := s.AddSkill(cmd.Context(), name, sourcePath); err != nil`,
	} {
		if !strings.Contains(cliHandlersContent, want) {
			t.Fatalf("generated CLI handlers missing %q:\n%s", want, cliHandlersContent)
		}
	}

	mcpSync, err := os.ReadFile(filepath.Join(outMCP, "sync.go"))
	if err != nil {
		t.Fatalf("read generated MCP sync file: %v", err)
	}
	if !strings.Contains(string(mcpSync), `const SyncToolName = "sync"`) {
		t.Fatalf("generated MCP sync file does not expose tool metadata:\n%s", string(mcpSync))
	}
	if !strings.Contains(string(mcpSync), `SyncToolParams = []string{"ctx", "opts"}`) {
		t.Fatalf("generated MCP sync file does not expose parameter metadata:\n%s", string(mcpSync))
	}

	mcpHandlers, err := os.ReadFile(filepath.Join(outMCP, "handlers.go"))
	if err != nil {
		t.Fatalf("read generated MCP handlers file: %v", err)
	}
	mcpHandlerContent := string(mcpHandlers)
	for _, want := range []string{
		`func GeneratedTools(s service.Service) []GeneratedTool`,
		`{Spec: SyncToolSpec(), Tool: SyncMCPTool(), Handler: SyncMCPHandler(s)}`,
		`options = append(options, mcplib.WithBoolean("dry_run"))`,
		`result, err := s.Sync(ctx, usecase.SyncOptions{Target: req.Target, DryRun: req.DryRun, Force: req.Force})`,
		`func AddSkillMCPHandler(s service.Service) ToolHandler`,
		`if strings.TrimSpace(req.Name) == ""`,
		`if err := s.AddSkill(ctx, req.Name, req.SourcePath); err != nil`,
	} {
		if !strings.Contains(mcpHandlerContent, want) {
			t.Fatalf("generated MCP handlers missing %q:\n%s", want, mcpHandlerContent)
		}
	}
	if strings.Contains(mcpHandlerContent, "switch spec.MethodName") {
		t.Fatalf("generated MCP handlers should not rely on a handwritten method switch:\n%s", mcpHandlerContent)
	}

	httpHandlers, err := os.ReadFile(filepath.Join(outHTTP, "handlers.go"))
	if err != nil {
		t.Fatalf("read generated HTTP handlers file: %v", err)
	}
	httpHandlerContent := string(httpHandlers)
	for _, want := range []string{
		`func GeneratedOperations(s service.Service) []GeneratedOperation`,
		`{Descriptor: mustOperation("Sync"), Handler: SyncHTTPHandler(s)}`,
		`result, err := s.Sync(ctx, usecase.SyncOptions{Target: req.Target, DryRun: req.DryRun, Force: req.Force})`,
		`func ListTargetsHTTPHandler(s service.Service) OperationHandler`,
		`result, err := s.ListTargets(ctx)`,
	} {
		if !strings.Contains(httpHandlerContent, want) {
			t.Fatalf("generated HTTP handlers missing %q:\n%s", want, httpHandlerContent)
		}
	}
	if strings.Contains(httpHandlerContent, "switch") {
		t.Fatalf("generated HTTP handlers should not rely on a handwritten method switch:\n%s", httpHandlerContent)
	}

	ops, err := os.ReadFile(filepath.Join(outOps, "operations.go"))
	if err != nil {
		t.Fatalf("read generated operation descriptors: %v", err)
	}
	opsContent := string(ops)
	for _, want := range []string{
		`MethodName:    "Sync"`,
		`OperationName: "sync"`,
		`CLIName:       "sync"`,
		`MCPName:       "sync"`,
		`HTTPRoute:     "/v1/operations/sync"`,
		`Name: "target", ExternalName: "target", Type: "string", Kind: "primitive", Required: false, CLIKind: "flag"`,
		`Name: "dryRun", ExternalName: "dry_run", Type: "bool", Kind: "primitive", Required: false, CLIKind: "flag"`,
		`Name: "force", ExternalName: "force", Type: "bool", Kind: "primitive", Required: false, CLIKind: "flag"`,
		`MethodName:    "AddSkill"`,
		`OperationName: "add_skill"`,
		`CLIName:       "add-skill"`,
		`Name: "name", ExternalName: "name", Type: "string", Kind: "primitive", Required: true, CLIKind: "arg"`,
		`Name: "sourcePath", ExternalName: "source_path", Type: "string", Kind: "primitive", Required: false, CLIKind: "arg"`,
		`MethodName:    "ListTargets"`,
		`OperationName: "list_targets"`,
	} {
		if !strings.Contains(opsContent, want) {
			t.Fatalf("generated operation descriptors missing %q:\n%s", want, opsContent)
		}
	}
	if strings.Contains(opsContent, `ExternalName: "context"`) {
		t.Fatalf("generated operation descriptors should not expose context.Context as an operation input:\n%s", opsContent)
	}
}

func TestServiceMethodsExtractsAllNamesCommentsAndParams(t *testing.T) {
	serviceFile := filepath.Join(t.TempDir(), "service.go")
	if err := os.WriteFile(serviceFile, []byte(`package fixture

import "context"

type Extra interface {
	// ExtraThing does extra work.
	ExtraThing(ctx context.Context) error
}

type Service interface {
	Extra
	// First does the first thing.
	First(ctx context.Context, name string) error
	Second(ctx context.Context) error
	Third(ctx context.Context) error
}
`), 0o644); err != nil {
		t.Fatalf("write service fixture: %v", err)
	}

	methods, err := serviceMethods(serviceFile)
	if err != nil {
		t.Fatalf("serviceMethods() error = %v", err)
	}

	want := map[string]struct {
		doc    string
		params []string
	}{
		"ExtraThing": {doc: "ExtraThing does extra work.", params: []string{"ctx"}},
		"First":      {doc: "First does the first thing.", params: []string{"ctx", "name"}},
		"Second":     {params: []string{"ctx"}},
		"Third":      {params: []string{"ctx"}},
	}
	if len(methods) != len(want) {
		t.Fatalf("got %d methods, want %d: %#v", len(methods), len(want), methods)
	}
	for _, method := range methods {
		w, ok := want[method.Name]
		if !ok {
			t.Fatalf("unexpected method %#v", method)
		}
		if w.doc != "" && method.Doc != w.doc {
			t.Fatalf("%s doc = %q, want %q", method.Name, method.Doc, w.doc)
		}
		if strings.Join(paramNamesFrom(method.Params), ",") != strings.Join(w.params, ",") {
			t.Fatalf("%s params = %#v, want %#v", method.Name, method.Params, w.params)
		}
	}
}

func TestOperationDescriptorContentCoversInputShapes(t *testing.T) {
	serviceFile := filepath.Join(t.TempDir(), "service.go")
	fixture := "package fixture\n\n" +
		"import \"context\"\n\n" +
		"type Options struct {\n\tName string `json:\"name\"`\n}\n\n" +
		"type Service interface {\n" +
		"\t// NoInput has no input after context.\n" +
		"\tNoInput(ctx context.Context) error\n" +
		"\t// SimpleParam uses a primitive input.\n" +
		"\tSimpleParam(ctx context.Context, name string) error\n" +
		"\t// StructParam uses a struct input.\n" +
		"\tStructParam(ctx context.Context, opts Options) (Result, error)\n" +
		"}\n\n" +
		"type Result struct{}\n"
	if err := os.WriteFile(serviceFile, []byte(fixture), 0o644); err != nil {
		t.Fatalf("write service fixture: %v", err)
	}

	methods, err := serviceMethods(serviceFile)
	if err != nil {
		t.Fatalf("serviceMethods() error = %v", err)
	}
	content, err := operationDescriptorContent(methods)
	if err != nil {
		t.Fatalf("operationDescriptorContent() error = %v", err)
	}
	for _, want := range []string{
		`OperationName: "no_input"`,
		`OperationName: "simple_param"`,
		`Name: "name", ExternalName: "name", Type: "string", Kind: "primitive", Required: true, CLIKind: "arg"`,
		`OperationName: "struct_param"`,
		`Name: "name", ExternalName: "name", Type: "string", Kind: "primitive", Required: true, CLIKind: "arg"`,
		`[]OutputDescriptor{{Name: "result1", Type: "Result"}, {Name: "result2", Type: "error"}}`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("descriptor content missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, `ExternalName: "context"`) {
		t.Fatalf("descriptor content should not expose context.Context as an operation input:\n%s", content)
	}
}

func TestHTTPHandlersContentUsesOnlyOwnedFieldsForStructParams(t *testing.T) {
	methods := []serviceMethod{
		{
			Name: "Foo",
			Params: []methodParam{
				{Name: "ctx", ExternalName: "context", Type: "context.Context", Kind: "context"},
				{Name: "id", ExternalName: "id", Type: "string", Kind: "primitive", Required: true},
				{Name: "opts", ExternalName: "opts", Type: "FooOptions", Kind: "struct", Fields: []methodParam{{Name: "name", ExternalName: "name", Type: "string", Kind: "primitive", Required: true}}},
			},
			Results: []methodResult{{Name: "result1", Type: "error"}},
		},
	}

	content, err := httpHandlersContent(methods)
	if err != nil {
		t.Fatalf("httpHandlersContent() error = %v", err)
	}
	for _, want := range []string{
		`type fooRequest struct`,
		`Id   string ` + "`json:\"id\"`",
		`Name string ` + "`json:\"name\"`",
		`if err := s.Foo(ctx, req.Id, service.FooOptions{Name: req.Name}); err != nil`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated HTTP handler content missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, `FooOptions{Id: req.Id`) {
		t.Fatalf("struct param included unrelated primitive field:\n%s", content)
	}
}

func TestMCPHandlersContentGeneratesCallableNewPrimitiveOperation(t *testing.T) {
	methods := []serviceMethod{
		{
			Name: "Ping",
			Doc:  "Ping checks generated tool delegation.",
			Params: []methodParam{
				{Name: "ctx", ExternalName: "context", Type: "context.Context", Kind: "context"},
				{Name: "message", ExternalName: "message", Type: "string", Kind: "primitive"},
				{Name: "count", ExternalName: "count", Type: "int", Kind: "primitive"},
			},
			Results: []methodResult{{Name: "result1", Type: "error"}},
		},
	}

	content, err := mcpHandlersContent(methods)
	if err != nil {
		t.Fatalf("mcpHandlersContent() error = %v", err)
	}
	for _, want := range []string{
		`{Spec: PingToolSpec(), Tool: PingMCPTool(), Handler: PingMCPHandler(s)}`,
		`type pingRequest struct`,
		`Message string ` + "`json:\"message\"`",
		`Count   int    ` + "`json:\"count,omitempty\"`",
		`options = append(options, mcplib.WithString("message", mcplib.Required()))`,
		`options = append(options, mcplib.WithInteger("count"))`,
		`if err := s.Ping(ctx, req.Message, req.Count); err != nil`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated MCP handler content missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "switch") {
		t.Fatalf("generated handler for a new operation should not require switch wiring:\n%s", content)
	}
}

func TestCLIHandlersContentGeneratesCallableNewPrimitiveOperation(t *testing.T) {
	methods := []serviceMethod{
		{
			Name: "Ping",
			Doc:  "Ping checks generated CLI delegation.",
			Params: []methodParam{
				{Name: "ctx", ExternalName: "context", Type: "context.Context", Kind: "context"},
				{Name: "message", ExternalName: "message", Type: "string", Kind: "primitive", Required: true, CLIKind: "arg"},
				{Name: "loud", ExternalName: "loud", Type: "bool", Kind: "primitive", CLIKind: "flag"},
			},
			Results: []methodResult{{Name: "result1", Type: "error"}},
		},
	}

	content, err := cliHandlersContent(methods)
	if err != nil {
		t.Fatalf("cliHandlersContent() error = %v", err)
	}
	for _, want := range []string{
		`func runPing(cmd *cobra.Command, s service.Service, args []string) error`,
		`message := positionalInput(args, 0)`,
		`loud, err := boolFlag(cmd, "loud")`,
		`return s.Ping(cmd.Context(), message, loud)`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("generated CLI handler content missing %q:\n%s", want, content)
		}
	}
}

func TestMCPHandlersContentRejectsUnexpandedStructOperation(t *testing.T) {
	methods := []serviceMethod{
		{
			Name: "StructParam",
			Params: []methodParam{
				{Name: "ctx", ExternalName: "context", Type: "context.Context", Kind: "context"},
				{Name: "opts", ExternalName: "opts", Type: "Options", Kind: "struct"},
				{Name: "name", ExternalName: "name", Type: "string", Kind: "primitive"},
			},
			Results: []methodResult{{Name: "result1", Type: "error"}},
		},
	}

	_, err := mcpHandlersContent(methods)
	if err == nil {
		t.Fatal("mcpHandlersContent() error = nil, want unexpanded struct input error")
	}
	if !strings.Contains(err.Error(), "struct inputs must be expanded into operation descriptor fields") {
		t.Fatalf("error = %v", err)
	}
}

func TestServiceMethodsValidatesProjectPackageStructTags(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/fixture\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	requestDir := filepath.Join(root, "internal", "dtos", "v2")
	serviceDir := filepath.Join(root, "internal", "service")
	if err := os.MkdirAll(requestDir, 0o755); err != nil {
		t.Fatalf("create request dir: %v", err)
	}
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("create service dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(requestDir, "options.go"), []byte("package requests\n\ntype SyncOptions struct {\n	Target string `json:\"target,omitempty\"`\n}\n"), 0o644); err != nil {
		t.Fatalf("write request options: %v", err)
	}
	serviceFile := filepath.Join(serviceDir, "service.go")
	if err := os.WriteFile(serviceFile, []byte("package service\n\nimport (\n	\"context\"\n	\"example.test/fixture/internal/dtos/v2\"\n)\n\ntype LocalRequest struct {\n	Name string `json:\"name\"`\n}\n\ntype Service interface {\n	Sync(ctx context.Context, opts requests.SyncOptions) error\n	Local(ctx context.Context, req LocalRequest) error\n}\n"), 0o644); err != nil {
		t.Fatalf("write service file: %v", err)
	}

	methods, err := serviceMethods(serviceFile)
	if err != nil {
		t.Fatalf("serviceMethods() error = %v", err)
	}
	if len(methods) != 2 {
		t.Fatalf("got %d methods, want 2: %#v", len(methods), methods)
	}
}

func TestServiceMethodsRejectsUnsupportedInputShapes(t *testing.T) {
	serviceFile := filepath.Join(t.TempDir(), "service.go")
	fixture := "package fixture\n\n" +
		"import \"context\"\n\n" +
		"type Base struct {\n	Name string\n}\n\n" +
		"type base struct {\n	Name string `json:\"name\"`\n}\n\n" +
		"type MissingTagsOptions struct {\n	Name string\n}\n\n" +
		"type EmbeddedRequest struct {\n	Base\n}\n\n" +
		"type UnexportedEmbeddedRequest struct {\n	base\n}\n\n" +
		"type TaggedRequest struct {\n	Name string `json:\"name\"`\n	private string\n}\n\n" +
		"type Service interface {\n" +
		"	Good(ctx context.Context, req TaggedRequest) error\n" +
		"	BadStruct(ctx context.Context, opts MissingTagsOptions) error\n" +
		"	BadEmbedded(ctx context.Context, req EmbeddedRequest) error\n" +
		"	BadUnexportedEmbedded(ctx context.Context, req UnexportedEmbeddedRequest) error\n" +
		"	BadSlice(ctx context.Context, names []string) error\n" +
		"}\n"
	if err := os.WriteFile(serviceFile, []byte(fixture), 0o644); err != nil {
		t.Fatalf("write service fixture: %v", err)
	}

	_, err := serviceMethods(serviceFile)
	if err == nil {
		t.Fatalf("serviceMethods() error = nil, want unsupported input shape error")
	}
	message := err.Error()
	for _, want := range []string{
		"service interface contains unsupported generated input shapes",
		"BadStruct.opts has unsupported input type MissingTagsOptions",
		"BadEmbedded.req has unsupported input type EmbeddedRequest",
		"BadUnexportedEmbedded.req has unsupported input type UnexportedEmbeddedRequest",
		"BadSlice.names has unsupported input type []string",
		"struct Options/Request params with json tags",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error missing %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "Good.req") {
		t.Fatalf("supported tagged request was rejected:\n%s", message)
	}
}

func TestNewOperationGoldenPathGeneratesCallableCLIMCPAndHTTP(t *testing.T) {
	fixtureRoot := filepath.Join(t.TempDir(), "golden")
	for _, dir := range []string{
		"internal/service",
		"internal/usecase",
		"internal/domain",
		"cmd/gen",
		"internal/mcp/gen",
		"internal/ops/gen",
		"internal/httpapi/gen",
	} {
		if err := os.MkdirAll(filepath.Join(fixtureRoot, dir), 0o755); err != nil {
			t.Fatalf("create fixture dir %s: %v", dir, err)
		}
	}
	writeFixtureFile(t, filepath.Join(fixtureRoot, "go.mod"), `module github.com/techgodhq/creed

go 1.26.2

require (
	github.com/mark3labs/mcp-go v0.55.1
	github.com/spf13/cobra v1.10.2
)
`)
	goSum, err := os.ReadFile(filepath.Join(repoRoot(t), "go.sum"))
	if err != nil {
		t.Fatalf("read repo go.sum: %v", err)
	}
	writeFixtureFile(t, filepath.Join(fixtureRoot, "go.sum"), string(goSum))
	writeFixtureFile(t, filepath.Join(fixtureRoot, "internal", "usecase", "usecase.go"), `package usecase

type SyncOptions struct {
	Target string `+"`json:\"target,omitempty\"`"+`
	DryRun bool   `+"`json:\"dry_run,omitempty\"`"+`
	Force  bool   `+"`json:\"force,omitempty\"`"+`
}

type TargetResult struct {
	Target          string
	FilesWritten    int
	FilesWouldWrite int
	FilesSkipped    int
	FilesFailed     int
	Files           []FileResult
}

type FileResult struct {
	Path   string
	Status string
}

type SyncResult struct {
	Targets []TargetResult
}

func (r *SyncResult) HasErrors() bool { return false }
`)
	writeFixtureFile(t, filepath.Join(fixtureRoot, "internal", "domain", "domain.go"), `package domain

type OutputKind string

const OutputKindContext OutputKind = "context"

type TargetOutput struct {
	Path   string
	Kind   OutputKind
	Format string
}

type TargetInfo struct {
	Name      string
	Enabled   bool
	OutputDir string
	EmitPaths []string
	Outputs   []TargetOutput
}
`)
	serviceFile := filepath.Join(fixtureRoot, "internal", "service", "service.go")
	writeFixtureFile(t, serviceFile, `package service

import (
	"context"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/usecase"
)

type PingRequest struct {
	Message string `+"`json:\"message\"`"+`
	Loud    bool   `+"`json:\"loud,omitempty\"`"+`
}

type PingResult struct {
	Message string `+"`json:\"message\"`"+`
	Loud    bool   `+"`json:\"loud\"`"+`
}

type Service interface {
	// Sync syncs configured targets.
	Sync(ctx context.Context, opts usecase.SyncOptions) (*usecase.SyncResult, error)
	// ListTargets lists configured targets.
	ListTargets(ctx context.Context) ([]domain.TargetInfo, error)
	// Ping proves a new DTO-backed operation is generated across all surfaces.
	Ping(ctx context.Context, req PingRequest) (PingResult, error)
}
`)

	if err := run([]string{
		"--service", serviceFile,
		"--out-cli", filepath.Join(fixtureRoot, "cmd", "gen"),
		"--out-mcp", filepath.Join(fixtureRoot, "internal", "mcp", "gen"),
		"--out-ops", filepath.Join(fixtureRoot, "internal", "ops", "gen"),
		"--out-http", filepath.Join(fixtureRoot, "internal", "httpapi", "gen"),
	}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	writeFixtureFile(t, filepath.Join(fixtureRoot, "golden_test.go"), `package creed_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	cligen "github.com/techgodhq/creed/cmd/gen"
	"github.com/techgodhq/creed/internal/domain"
	httpgen "github.com/techgodhq/creed/internal/httpapi/gen"
	mcpgen "github.com/techgodhq/creed/internal/mcp/gen"
	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
)

func TestGeneratedSurfacesDelegateNewOperation(t *testing.T) {
	svc := &fakeService{}

	var cmdFound bool
	for _, command := range cligen.Commands(svc) {
		if command.Name() != "ping" {
			continue
		}
		cmdFound = true
		var out bytes.Buffer
		command.SetOut(&out)
		command.SetArgs([]string{"hello", "--loud"})
		if err := command.Execute(); err != nil {
			t.Fatalf("generated CLI ping command from registry: %v", err)
		}
		break
	}
	if !cmdFound {
		t.Fatalf("generated CLI commands missing ping")
	}
	if svc.cliMessage != "hello" || !svc.cliLoud {
		t.Fatalf("CLI ping call = (%q, %t)", svc.cliMessage, svc.cliLoud)
	}

	mcpTools := mcpgen.GeneratedTools(svc)
	mcpHandler, ok := mcpToolHandler(mcpTools, "ping")
	if !ok {
		t.Fatalf("generated MCP tools missing ping: %#v", mcpTools)
	}
	if _, err := mcpHandler(context.Background(), json.RawMessage([]byte("{\"message\":\"mcp\",\"loud\":true}"))); err != nil {
		t.Fatalf("generated MCP ping handler from registry: %v", err)
	}
	if svc.mcpMessage != "mcp" || !svc.mcpLoud {
		t.Fatalf("MCP ping call = (%q, %t)", svc.mcpMessage, svc.mcpLoud)
	}

	httpOps := httpgen.GeneratedOperations(svc)
	httpHandler, ok := httpOperationHandler(httpOps, "ping")
	if !ok {
		t.Fatalf("generated HTTP operations missing ping: %#v", httpOps)
	}
	result, err := httpHandler(context.Background(), json.RawMessage([]byte("{\"message\":\"http\",\"loud\":true}")))
	if err != nil {
		t.Fatalf("generated HTTP ping handler: %v", err)
	}
	pingResult, ok := result.(service.PingResult)
	if !ok || pingResult.Message != "http" || !pingResult.Loud {
		t.Fatalf("HTTP ping result = %#v", result)
	}
	if svc.httpMessage != "http" || !svc.httpLoud {
		t.Fatalf("HTTP ping call = (%q, %t)", svc.httpMessage, svc.httpLoud)
	}
}

type fakeService struct {
	cliMessage  string
	cliLoud     bool
	mcpMessage  string
	mcpLoud     bool
	httpMessage string
	httpLoud    bool
	calls       int
}

func (f *fakeService) Sync(ctx context.Context, opts usecase.SyncOptions) (*usecase.SyncResult, error) {
	return &usecase.SyncResult{}, nil
}

func (f *fakeService) ListTargets(ctx context.Context) ([]domain.TargetInfo, error) {
	return []domain.TargetInfo{{Name: "codex", Enabled: true, OutputDir: ".", EmitPaths: []string{"AGENTS.md"}, Outputs: []domain.TargetOutput{{Path: "AGENTS.md", Kind: domain.OutputKindContext, Format: "markdown"}}}}, nil
}

func (f *fakeService) Ping(ctx context.Context, req service.PingRequest) (service.PingResult, error) {
	f.calls++
	switch f.calls {
	case 1:
		f.cliMessage, f.cliLoud = req.Message, req.Loud
	case 2:
		f.mcpMessage, f.mcpLoud = req.Message, req.Loud
	case 3:
		f.httpMessage, f.httpLoud = req.Message, req.Loud
	}
	return service.PingResult{Message: req.Message, Loud: req.Loud}, nil
}

func mcpToolHandler(tools []mcpgen.GeneratedTool, name string) (mcpgen.ToolHandler, bool) {
	for _, tool := range tools {
		if tool.Spec.Name == name {
			return tool.Handler, true
		}
	}
	return nil, false
}

func httpOperationHandler(operations []httpgen.GeneratedOperation, name string) (httpgen.OperationHandler, bool) {
	for _, operation := range operations {
		if operation.Descriptor.OperationName == name {
			return operation.Handler, true
		}
	}
	return nil, false
}
`)

	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = fixtureRoot
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("golden fixture go test failed: %v\n%s", err, output)
	}
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture file %s: %v", path, err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s to exist: %v", path, err)
	}
}
