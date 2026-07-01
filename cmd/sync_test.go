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

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir temp project: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	}()

	var out bytes.Buffer
	syncDryRun = false
	syncForce = false
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs([]string{"sync", "--target", "claude", "--dry-run"})
	defer func() {
		rootCmd.SetOut(os.Stdout)
		rootCmd.SetErr(os.Stderr)
		rootCmd.SetArgs(nil)
		syncDryRun = false
		syncForce = false
	}()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("sync command failed: %v\noutput:\n%s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "claude: 0 written, 1 would_write, 0 skipped, 0 failed") {
		t.Fatalf("dry-run summary did not include would_write count; output:\n%s", output)
	}
	if !strings.Contains(output, "  would_write CLAUDE.md") {
		t.Fatalf("dry-run output should list would-write files; output:\n%s", output)
	}
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
