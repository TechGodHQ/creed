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

	mcpSync, err := os.ReadFile(filepath.Join(outMCP, "sync.go"))
	if err != nil {
		t.Fatalf("read generated MCP sync file: %v", err)
	}
	if !strings.Contains(string(mcpSync), `const SyncToolName = "sync"`) {
		t.Fatalf("generated MCP sync file does not expose tool metadata:\n%s", string(mcpSync))
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
