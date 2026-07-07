package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGeneratesCLIAndMCPFilesForServiceMethods(t *testing.T) {
	root := repoRoot(t)
	outCLI := filepath.Join(t.TempDir(), "cmd", "gen")
	outMCP := filepath.Join(t.TempDir(), "internal", "mcp", "gen")

	err := run([]string{
		"--service", filepath.Join(root, "internal", "service", "service.go"),
		"--out-cli", outCLI,
		"--out-mcp", outMCP,
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
	cliSync, err := os.ReadFile(filepath.Join(outCLI, "sync.go"))
	if err != nil {
		t.Fatalf("read generated CLI sync file: %v", err)
	}
	if !strings.Contains(string(cliSync), "func NewSyncCommand(s service.Service) *cobra.Command") {
		t.Fatalf("generated CLI sync file does not expose NewSyncCommand wrapper:\n%s", string(cliSync))
	}
	if !strings.Contains(string(cliSync), `Use:   "sync"`) {
		t.Fatalf("generated CLI sync file does not use snake_case command name:\n%s", string(cliSync))
	}
	if !strings.Contains(string(cliSync), `Short: "Sync syncs configured Creed context to one or more targets."`) {
		t.Fatalf("generated CLI sync file does not use Service doc comment:\n%s", string(cliSync))
	}
	if !strings.Contains(string(cliSync), `ParamNames: []string{"ctx", "opts"}`) {
		t.Fatalf("generated CLI sync file does not expose parameter metadata:\n%s", string(cliSync))
	}
	if strings.Contains(string(cliSync), "not wired yet") {
		t.Fatalf("generated CLI sync file should not emit unwired runtime errors:\n%s", string(cliSync))
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
}

func TestServiceMethodsExtractsAllNamesCommentsAndParams(t *testing.T) {
	serviceFile := filepath.Join(t.TempDir(), "service.go")
	if err := os.WriteFile(serviceFile, []byte(`package fixture

type Extra interface {
	// ExtraThing does extra work.
	ExtraThing(ctx Context) error
}

type Service interface {
	Extra
	// First does the first thing.
	First(ctx Context, name string) error
	Second(ctx Context) error
	Third(ctx Context) error
}

type Context struct{}
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
		if strings.Join(method.Params, ",") != strings.Join(w.params, ",") {
			t.Fatalf("%s params = %#v, want %#v", method.Name, method.Params, w.params)
		}
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
