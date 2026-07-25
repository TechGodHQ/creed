package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	for _, name := range []string{"init", "sync", "add-skill", "remove-skill", "list-skills", "list-targets", "enable-target", "disable-target", "pull", "push", "watch"} {
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
	if !strings.Contains(output, "claude	enabled	.") {
		t.Fatalf("list-targets should include service-derived claude target state; output:\n%s", output)
	}
	for _, want := range []string{
		"claude	enabled	.	CLAUDE.md,.claude/skills/	CLAUDE.md|context|markdown,.claude/skills/|skill_dir|markdown",
		"copilot	disabled		.github/copilot-instructions.md	.github/copilot-instructions.md|context|markdown",
		"cursor	disabled		.cursor/rules/	.cursor/rules/|skill_dir|markdown",
		"gemini	disabled		GEMINI.md,.gemini/	GEMINI.md|context|markdown,.gemini/|skill_dir|markdown",
		"opencode	disabled		AGENTS.md,.opencode/agents/	AGENTS.md|context|markdown,.opencode/agents/|skill_dir|markdown",
		"aider	disabled		.aider.conf.yml,CONVENTIONS.md	.aider.conf.yml|config|yaml,CONVENTIONS.md|context|markdown",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("list-targets should expose stable output descriptors %q; output:\n%s", want, output)
		}
	}
}

// TestWatchCommandSyncsOnSourceChange drives the generated `creed watch`
// command end-to-end: it starts the watcher, mutates a canonical source
// file, and confirms that a sync ran (the emitted CLAUDE.md picks up the
// change). It then cancels the watch via SIGINT to confirm clean exit.
func TestWatchCommandSyncsOnSourceChange(t *testing.T) {
	if testing.Short() {
		t.Skip("watch test exercises real fsnotify and is slow-path")
	}
	projectDir := t.TempDir()
	writeTestCreedProject(t, projectDir)

	// Run the watch command in a goroutine; it blocks until ctx cancel.
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	// Use a mutex-guarded writer so the test goroutine can safely read
	// output while the watch goroutine is still appending to it.
	safeOut := newSafeBuffer()
	rootCmd.SetOut(safeOut)
	rootCmd.SetErr(safeOut)
	rootCmd.SetArgs([]string{"watch", "--target", "claude", "--debounce", "50ms"})
	defer rootCmd.SetArgs(nil)

	watchErr := make(chan error, 1)
	go func() {
		watchErr <- rootCmd.Execute()
	}()

	// Wait for the watcher startup banner so we know fsnotify is live.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(safeOut.String(), "watching .creed/") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(safeOut.String(), "watching .creed/") {
		t.Fatalf("watch command did not announce startup; output:\n%s", safeOut.String())
	}

	// Mutate a canonical source file.
	configPath := filepath.Join(projectDir, ".creed", "config", "project.md")
	if err := os.WriteFile(configPath, []byte("# Project\n\nWatch-triggered update.\n"), 0o644); err != nil {
		t.Fatalf("rewrite project.md: %v", err)
	}

	// Wait for the sync to land on disk via the emitted CLAUDE.md.
	deadline = time.Now().Add(3 * time.Second)
	synced := false
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md"))
		if err == nil && strings.Contains(string(data), "Watch-triggered update.") {
			synced = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !synced {
		t.Fatalf("watch did not sync the source change to CLAUDE.md; output:\n%s", safeOut.String())
	}

	// Cancel via SIGINT to confirm clean exit.
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find process: %v", err)
	}
	_ = proc.Signal(os.Interrupt)

	select {
	case <-watchErr:
	case <-time.After(3 * time.Second):
		t.Fatal("watch command did not exit within 3s of SIGINT")
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
