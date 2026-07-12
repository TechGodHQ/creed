package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techgodhq/creed/internal/adapters/localfs"
	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/usecase"
)

func TestInitCreatesStarterScaffoldAndPracticalDefaultTargets(t *testing.T) {
	root := t.TempDir()
	svc := New(root)

	if err := svc.Init(context.Background(), "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".creed", "manifest.yaml")); err != nil {
		t.Fatalf("manifest was not created: %v", err)
	}
	for _, path := range []string{
		filepath.Join(root, ".creed", "config", "project.md"),
		filepath.Join(root, ".creed", "config", "development.md"),
		filepath.Join(root, ".creed", "skills", "review.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("scaffold file %s was not created: %v", path, err)
		}
	}

	targets, err := svc.ListTargets(context.Background())
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	enabled := map[string]bool{}
	for _, target := range targets {
		if target.OutputDir != "." {
			t.Fatalf("target %s OutputDir = %q, want .", target.Name, target.OutputDir)
		}
		enabled[target.Name] = target.Enabled
	}
	for _, name := range []string{"claude", "codex", "cursor"} {
		if !enabled[name] {
			t.Fatalf("target %s should default to enabled; enabled=%#v", name, enabled)
		}
	}
	for _, name := range []string{"agents", "aider", "windsurf"} {
		if enabled[name] {
			t.Fatalf("target %s should default to disabled; enabled=%#v", name, enabled)
		}
	}
	skills, err := svc.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "review" || skills[0].Path != "skills/review.md" {
		t.Fatalf("default skills = %#v, want review skill", skills)
	}
	configs, err := localfs.NewSource(root).ListConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}
	if len(configs) != 2 || configs[0].Name != "project" || configs[1].Name != "development" {
		t.Fatalf("default configs = %#v, want project and development", configs)
	}
}

func TestEmitPathsFromOutputsDerivesOrderedPaths(t *testing.T) {
	outputs := []domain.TargetOutput{
		{Path: ".aider.conf.yml", Kind: domain.OutputKindConfig, Format: "yaml"},
		{Path: "CONVENTIONS.md", Kind: domain.OutputKindContext, Format: "markdown"},
	}

	paths := emitPathsFromOutputs(outputs)

	if got, want := strings.Join(paths, ","), ".aider.conf.yml,CONVENTIONS.md"; got != want {
		t.Fatalf("emitPathsFromOutputs() = %q, want %q", got, want)
	}
	outputs[0].Path = "mutated"
	if paths[0] != ".aider.conf.yml" {
		t.Fatalf("emitPathsFromOutputs() returned paths aliased to outputs: %#v", paths)
	}
}

func TestListTargetsExposesStructuredOutputDescriptors(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	targets, err := svc.ListTargets(ctx)
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if len(targets) == 0 {
		t.Fatal("ListTargets() returned no targets")
	}

	for _, target := range targets {
		t.Run(target.Name, func(t *testing.T) {
			if len(target.Outputs) == 0 {
				t.Fatalf("target %s Outputs is empty", target.Name)
			}
			if len(target.EmitPaths) != len(target.Outputs) {
				t.Fatalf("target %s EmitPaths len = %d, Outputs len = %d", target.Name, len(target.EmitPaths), len(target.Outputs))
			}
			for i, output := range target.Outputs {
				if output.Path == "" {
					t.Fatalf("target %s output %d has empty path: %#v", target.Name, i, output)
				}
				if output.Kind == "" {
					t.Fatalf("target %s output %d has empty kind: %#v", target.Name, i, output)
				}
				if output.Format == "" {
					t.Fatalf("target %s output %d has empty format: %#v", target.Name, i, output)
				}
				if target.EmitPaths[i] != output.Path {
					t.Fatalf("target %s EmitPaths[%d] = %q, Outputs[%d].Path = %q", target.Name, i, target.EmitPaths[i], i, output.Path)
				}
			}
		})
	}

	for _, tt := range []struct {
		name string
		want []domain.TargetOutput
	}{
		{
			name: "aider",
			want: []domain.TargetOutput{
				{Path: ".aider.conf.yml", Kind: domain.OutputKindConfig, Format: "yaml"},
				{Path: "CONVENTIONS.md", Kind: domain.OutputKindContext, Format: "markdown"},
			},
		},
		{
			name: "claude",
			want: []domain.TargetOutput{
				{Path: "CLAUDE.md", Kind: domain.OutputKindContext, Format: "markdown"},
				{Path: ".claude/skills/", Kind: domain.OutputKindSkillDir, Format: "markdown"},
			},
		},
		{
			name: "cursor",
			want: []domain.TargetOutput{
				{Path: ".cursor/rules/", Kind: domain.OutputKindSkillDir, Format: "markdown"},
			},
		},
	} {
		t.Run("exact-"+tt.name, func(t *testing.T) {
			target, ok := findTargetInfo(targets, tt.name)
			if !ok {
				t.Fatalf("target %s not found in %#v", tt.name, targets)
			}
			if len(target.Outputs) != len(tt.want) {
				t.Fatalf("target %s Outputs len = %d, want %d: %#v", tt.name, len(target.Outputs), len(tt.want), target.Outputs)
			}
			for i, want := range tt.want {
				if got := target.Outputs[i]; got != want {
					t.Fatalf("target %s Outputs[%d] = %#v, want %#v", tt.name, i, got, want)
				}
			}
		})
	}
}

func TestInitPreservesExistingScaffoldFiles(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	customProject := "# Custom project\nDo not overwrite me.\n"
	customReview := "# Custom review\nKeep this.\n"
	if err := os.WriteFile(filepath.Join(root, ".creed", "config", "project.md"), []byte(customProject), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".creed", "skills", "review.md"), []byte(customReview), 0644); err != nil {
		t.Fatal(err)
	}

	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("second Init() error = %v", err)
	}

	if got := mustRead(t, filepath.Join(root, ".creed", "config", "project.md")); got != customProject {
		t.Fatalf("project scaffold overwritten: got %q, want %q", got, customProject)
	}
	if got := mustRead(t, filepath.Join(root, ".creed", "skills", "review.md")); got != customReview {
		t.Fatalf("review scaffold overwritten: got %q, want %q", got, customReview)
	}
}

func TestAddRemoveSkillMutatesManifest(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := svc.AddSkill(ctx, "testing", "skills/testing.md"); err != nil {
		t.Fatalf("AddSkill() error = %v", err)
	}
	skills, err := svc.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if len(skills) != 2 || skills[1].Name != "testing" || skills[1].Path != "skills/testing.md" {
		t.Fatalf("ListSkills() = %#v, want default review plus testing skill", skills)
	}
	if err := svc.AddSkill(ctx, "testing", "skills/testing-v2.md"); err != nil {
		t.Fatalf("AddSkill(update) error = %v", err)
	}
	skills, err = svc.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills(update) error = %v", err)
	}
	if len(skills) != 2 || skills[1].Path != "skills/testing-v2.md" {
		t.Fatalf("updated skills = %#v, want one updated entry", skills)
	}
	if err := svc.RemoveSkill(ctx, "testing"); err != nil {
		t.Fatalf("RemoveSkill() error = %v", err)
	}
	skills, err = svc.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills(after remove) error = %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "review" {
		t.Fatalf("skills after remove = %#v, want default review only", skills)
	}
}

func TestEnableDisableTargetAndSync(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writeProjectConfig(t, root)

	result, err := svc.Sync(ctx, usecase.SyncOptions{Target: "codex"})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.TotalFilesWritten() != 1 {
		t.Fatalf("TotalFilesWritten() = %d, want 1; result=%#v", result.TotalFilesWritten(), result)
	}
	if got := mustRead(t, filepath.Join(root, "AGENTS.md")); !strings.Contains(got, "# Project\n") {
		t.Fatalf("AGENTS.md = %q, want project content", got)
	}
	if err := svc.DisableTarget(ctx, "codex"); err != nil {
		t.Fatalf("DisableTarget() error = %v", err)
	}
	targets, err := svc.ListTargets(ctx)
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	for _, target := range targets {
		if target.Name == "codex" && target.Enabled {
			t.Fatal("codex target should be disabled")
		}
	}
}

func TestSyncHonorsTargetOutputDirForFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writeProjectConfig(t, root)
	writeSkill(t, root, "review", "# Review\n")
	manifest := mustRead(t, filepath.Join(root, ".creed", "manifest.yaml"))
	manifest = strings.Replace(manifest, "name: codex\n      enabled: true\n      output_dir: .", "name: codex\n      enabled: true\n      output_dir: generated", 1)
	manifest = strings.Replace(manifest, "name: cursor\n      enabled: true\n      output_dir: .", "name: cursor\n      enabled: true\n      output_dir: generated", 1)
	if err := os.WriteFile(filepath.Join(root, ".creed", "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sync(ctx, usecase.SyncOptions{}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if got := mustRead(t, filepath.Join(root, "generated", "AGENTS.md")); !strings.Contains(got, "# Project\n") {
		t.Fatalf("generated AGENTS.md = %q, want project content", got)
	}
	if got := mustRead(t, filepath.Join(root, "generated", ".cursor", "rules", "review.md")); got != "# Review\n" {
		t.Fatalf("generated cursor skill = %q, want skill content", got)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("root AGENTS.md exists or stat failed unexpectedly: %v", err)
	}
}

func TestSyncRejectsEscapingOutputDir(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	writeProjectConfig(t, root)
	manifest := mustRead(t, filepath.Join(root, ".creed", "manifest.yaml"))
	manifest = strings.Replace(manifest, "name: codex\n      enabled: true\n      output_dir: .", "name: codex\n      enabled: true\n      output_dir: ../outside", 1)
	if err := os.WriteFile(filepath.Join(root, ".creed", "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sync(ctx, usecase.SyncOptions{}); err == nil {
		t.Fatal("Sync() error = nil, want escaping output_dir error")
	}
}

func TestPushPublishesCreedDirToGitRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not available")
	}
	root := t.TempDir()
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitInit := exec.Command("git", "init", "--bare", remote)
	if output, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, output)
	}
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := svc.Push(ctx, remote); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if err := svc.AddSkill(ctx, "review", "skills/review.md"); err != nil {
		t.Fatalf("AddSkill() error = %v", err)
	}
	writeSkill(t, root, "review", "# Review\n")
	if err := svc.Push(ctx, remote); err != nil {
		t.Fatalf("Push(second) error = %v", err)
	}
	verifyDir := filepath.Join(t.TempDir(), "verify")
	clone := exec.Command("git", "clone", remote, verifyDir)
	if output, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v: %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(verifyDir, ".creed", "manifest.yaml")); err != nil {
		t.Fatalf("pushed manifest missing: %v", err)
	}
	if got := mustRead(t, filepath.Join(verifyDir, ".creed", "skills", "review.md")); got != "# Review\n" {
		t.Fatalf("pushed skill = %q, want review skill", got)
	}
}

func writeProjectConfig(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".creed", "config"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".creed", "config", "project.md"), []byte("# Project\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeSkill(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".creed", "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".creed", "skills", name+".md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func findTargetInfo(targets []domain.TargetInfo, name string) (domain.TargetInfo, bool) {
	for _, target := range targets {
		if target.Name == name {
			return target, true
		}
	}
	return domain.TargetInfo{}, false
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
