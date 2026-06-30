package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techgodhq/creed/internal/usecase"
)

func TestInitCreatesManifestWithDefaultTargets(t *testing.T) {
	root := t.TempDir()
	svc := New(root)

	if err := svc.Init(context.Background(), "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".creed", "manifest.yaml")); err != nil {
		t.Fatalf("manifest was not created: %v", err)
	}
	targets, err := svc.ListTargets(context.Background())
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if len(targets) == 0 {
		t.Fatal("expected default targets")
	}
	for _, target := range targets {
		if target.Enabled {
			t.Fatalf("target %s should default to disabled", target.Name)
		}
		if target.OutputDir != "." {
			t.Fatalf("target %s OutputDir = %q, want .", target.Name, target.OutputDir)
		}
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
	if len(skills) != 1 || skills[0].Name != "testing" || skills[0].Path != "skills/testing.md" {
		t.Fatalf("ListSkills() = %#v, want testing skill", skills)
	}
	if err := svc.AddSkill(ctx, "testing", "skills/testing-v2.md"); err != nil {
		t.Fatalf("AddSkill(update) error = %v", err)
	}
	skills, err = svc.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills(update) error = %v", err)
	}
	if len(skills) != 1 || skills[0].Path != "skills/testing-v2.md" {
		t.Fatalf("updated skills = %#v, want one updated entry", skills)
	}
	if err := svc.RemoveSkill(ctx, "testing"); err != nil {
		t.Fatalf("RemoveSkill() error = %v", err)
	}
	skills, err = svc.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills(after remove) error = %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("skills after remove = %#v, want empty", skills)
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
	manifest := mustRead(t, filepath.Join(root, ".creed", "manifest.yaml"))
	manifest += "config:\n  - name: project\n    path: config/project.md\n"
	if err := os.WriteFile(filepath.Join(root, ".creed", "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnableTarget(ctx, "codex"); err != nil {
		t.Fatalf("EnableTarget() error = %v", err)
	}

	result, err := svc.Sync(ctx, usecase.SyncOptions{})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.TotalFilesWritten() != 1 {
		t.Fatalf("TotalFilesWritten() = %d, want 1; result=%#v", result.TotalFilesWritten(), result)
	}
	if got := mustRead(t, filepath.Join(root, "AGENTS.md")); got != "# Project\n" {
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
	manifest += "config:\n  - name: project\n    path: config/project.md\nskills:\n  - name: review\n    path: skills/review.md\n"
	if err := os.WriteFile(filepath.Join(root, ".creed", "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnableTarget(ctx, "codex"); err != nil {
		t.Fatalf("EnableTarget(codex) error = %v", err)
	}
	if err := svc.EnableTarget(ctx, "cursor"); err != nil {
		t.Fatalf("EnableTarget(cursor) error = %v", err)
	}
	manifest = mustRead(t, filepath.Join(root, ".creed", "manifest.yaml"))
	manifest = strings.Replace(manifest, "name: codex\n      enabled: true\n      output_dir: .", "name: codex\n      enabled: true\n      output_dir: generated", 1)
	manifest = strings.Replace(manifest, "name: cursor\n      enabled: true\n      output_dir: .", "name: cursor\n      enabled: true\n      output_dir: generated", 1)
	if err := os.WriteFile(filepath.Join(root, ".creed", "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sync(ctx, usecase.SyncOptions{}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if got := mustRead(t, filepath.Join(root, "generated", "AGENTS.md")); got != "# Project\n" {
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
	manifest += "config:\n  - name: project\n    path: config/project.md\n"
	if err := os.WriteFile(filepath.Join(root, ".creed", "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnableTarget(ctx, "codex"); err != nil {
		t.Fatalf("EnableTarget() error = %v", err)
	}
	manifest = mustRead(t, filepath.Join(root, ".creed", "manifest.yaml"))
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
	verifyDir := filepath.Join(t.TempDir(), "verify")
	clone := exec.Command("git", "clone", remote, verifyDir)
	if output, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v: %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(verifyDir, ".creed", "manifest.yaml")); err != nil {
		t.Fatalf("pushed manifest missing: %v", err)
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

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
