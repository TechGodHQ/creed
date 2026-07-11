package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncCommandDryRunSummaryIncludesWouldWriteCount(t *testing.T) {
	projectDir := t.TempDir()
	writeTestCreedProject(t, projectDir)

	out := executeRootCommandInDir(t, projectDir, "sync", "--target", "claude", "--dry-run")

	output := out.String()
	if !strings.Contains(output, "claude: 0 written, 1 would_write, 0 skipped, 0 failed") {
		t.Fatalf("dry-run summary did not include would_write count; output:\n%s", output)
	}
	if !strings.Contains(output, "  would_write CLAUDE.md") {
		t.Fatalf("dry-run output should list would-write files; output:\n%s", output)
	}
}

func TestInitCommandCreatesProjectScaffold(t *testing.T) {
	projectDir := t.TempDir()

	out := executeRootCommandInDir(t, projectDir, "init", "demo")

	if !strings.Contains(out.String(), "Initialized creed project") {
		t.Fatalf("init output should report created project; output:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".creed", "manifest.yaml")); err != nil {
		t.Fatalf("init should create manifest: %v", err)
	}
}

func TestGeneratedCommandsAreRegisteredWithoutConflictingHandwrittenCommands(t *testing.T) {
	for _, name := range []string{"init", "sync", "add-skill", "remove-skill", "list-skills", "list-targets", "enable-target", "disable-target", "pull", "push"} {
		matches := 0
		for _, command := range rootCmd.Commands() {
			if command.Name() == name {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("expected exactly one %q command, got %d", name, matches)
		}
	}
}

func TestGeneratedListTargetsCommandDelegatesToService(t *testing.T) {
	projectDir := t.TempDir()
	writeTestCreedProject(t, projectDir)

	out := executeRootCommandInDir(t, projectDir, "list-targets")

	output := out.String()
	if !strings.Contains(output, "claude\tenabled\t.") {
		t.Fatalf("list-targets should include service-derived claude target state; output:\n%s", output)
	}
}

func executeRootCommandInDir(t *testing.T, dir string, args ...string) bytes.Buffer {
	t.Helper()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp project: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	defer func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		rootCmd.SetArgs(nil)
	}()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("command %v failed: %v\noutput:\n%s", args, err, out.String())
	}
	return out
}

func writeTestCreedProject(t *testing.T, projectDir string) {
	t.Helper()
	creedDir := filepath.Join(projectDir, ".creed")
	if err := os.MkdirAll(filepath.Join(creedDir, "config"), 0o755); err != nil {
		t.Fatalf("create creed config dir: %v", err)
	}
	manifest := `version: 1
source:
  type: local
  path: .creed
targets:
  - name: claude
    enabled: true
    output_dir: .
config:
  - name: project
    path: config/project.md
`
	if err := os.WriteFile(filepath.Join(creedDir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(creedDir, "config", "project.md"), []byte("# Project\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}
