package localfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// createTestProject sets up a temporary .creed/ directory with a manifest and skill files.
func createTestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	creedDir := filepath.Join(root, ".creed")
	if err := os.MkdirAll(filepath.Join(creedDir, "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(creedDir, "config"), 0755); err != nil {
		t.Fatal(err)
	}

	manifest := `version: 1
source:
  type: local
  path: .creed

targets:
  - name: claude
    enabled: true
    output_dir: .

skills:
  - name: code-review
    path: skills/code-review.md
  - name: testing
    path: skills/testing.md

config:
  - name: project-context
    path: config/project.md
`
	if err := os.WriteFile(filepath.Join(creedDir, "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creedDir, "skills", "code-review.md"), []byte("# Code Review\nReview code."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creedDir, "skills", "testing.md"), []byte("# Testing\nWrite tests."), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creedDir, "config", "project.md"), []byte("# Project\nContext here."), 0644); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestReadManifest(t *testing.T) {
	root := createTestProject(t)
	src := NewSource(root)

	ctx := context.Background()
	m, err := src.ReadManifest(ctx)
	if err != nil {
		t.Fatalf("ReadManifest error: %v", err)
	}
	if m.Version != 1 {
		t.Errorf("expected Version == 1, got %d", m.Version)
	}
	if m.Source.Type != "local" {
		t.Errorf("expected Source.Type == \"local\", got %q", m.Source.Type)
	}
	if len(m.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(m.Skills))
	}
	if len(m.Configs) != 1 {
		t.Errorf("expected 1 config, got %d", len(m.Configs))
	}
	if len(m.Targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(m.Targets))
	}
	if m.Targets[0].Name != "claude" || !m.Targets[0].Enabled {
		t.Errorf("unexpected target config: %+v", m.Targets[0])
	}
}

func TestReadManifestMissing(t *testing.T) {
	root := t.TempDir()
	src := NewSource(root)

	_, err := src.ReadManifest(context.Background())
	if err == nil {
		t.Fatal("expected error for missing manifest, got nil")
	}
}

func TestReadManifestMalformed(t *testing.T) {
	root := t.TempDir()
	creedDir := filepath.Join(root, ".creed")
	if err := os.MkdirAll(creedDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Use YAML that produces a type error: "version" expects a scalar
	// but we provide a mapping, which causes a yaml unmarshal error.
	malformed := []byte("version:\n  - a: b\n  nested: [1, 2, 3]\nsource:\n  type: []\n")
	if err := os.WriteFile(filepath.Join(creedDir, "manifest.yaml"), malformed, 0644); err != nil {
		t.Fatal(err)
	}

	src := NewSource(root)
	_, err := src.ReadManifest(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed manifest, got nil")
	}
}

func TestReadSkill(t *testing.T) {
	root := createTestProject(t)
	src := NewSource(root)

	skill, err := src.ReadSkill(context.Background(), "code-review")
	if err != nil {
		t.Fatalf("ReadSkill error: %v", err)
	}
	if skill.Name != "code-review" {
		t.Errorf("expected Name == \"code-review\", got %q", skill.Name)
	}
	if string(skill.Content) != "# Code Review\nReview code." {
		t.Errorf("unexpected content: %q", string(skill.Content))
	}
}

func TestReadSkillMissing(t *testing.T) {
	root := createTestProject(t)
	src := NewSource(root)

	_, err := src.ReadSkill(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing skill, got nil")
	}
}

func TestListSkills(t *testing.T) {
	root := createTestProject(t)
	src := NewSource(root)

	skills, err := src.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("ListSkills error: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}
	// Skills should be in manifest order
	if skills[0].Name != "code-review" {
		t.Errorf("expected skills[0].Name == \"code-review\", got %q", skills[0].Name)
	}
	if skills[1].Name != "testing" {
		t.Errorf("expected skills[1].Name == \"testing\", got %q", skills[1].Name)
	}
}

func TestReadConfig(t *testing.T) {
	root := createTestProject(t)
	src := NewSource(root)

	cfg, err := src.ReadConfig(context.Background(), "project-context")
	if err != nil {
		t.Fatalf("ReadConfig error: %v", err)
	}
	if cfg.Name != "project-context" {
		t.Errorf("expected Name == \"project-context\", got %q", cfg.Name)
	}
	if string(cfg.Content) != "# Project\nContext here." {
		t.Errorf("unexpected content: %q", string(cfg.Content))
	}
}

func TestReadConfigMissing(t *testing.T) {
	root := createTestProject(t)
	src := NewSource(root)

	_, err := src.ReadConfig(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing config, got nil")
	}
}

func TestListConfigs(t *testing.T) {
	root := createTestProject(t)
	src := NewSource(root)

	configs, err := src.ListConfigs(context.Background())
	if err != nil {
		t.Fatalf("ListConfigs error: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Name != "project-context" {
		t.Errorf("expected configs[0].Name == \"project-context\", got %q", configs[0].Name)
	}
}

func TestReadConfigRejectsSymlinkEscape(t *testing.T) {
	root := createTestProject(t)
	external := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(external, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, ".creed", "config", "leak.md")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, ".creed", "manifest.yaml")
	manifest := mustReadLocal(t, manifestPath)
	manifest += "  - name: leak\n    path: config/leak.md\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSource(root).ReadConfig(context.Background(), "leak"); err == nil {
		t.Fatal("ReadConfig followed a symlink outside the source directory")
	}
}

func mustReadLocal(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReadManifestRejectsAncestorSymlinkSourcePath(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, ".creed"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := "version: 1\nsource:\n  type: local\n  path: .creed\n"
	if err := os.WriteFile(filepath.Join(outside, ".creed", "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSourceWithPath(root, "link/.creed").ReadManifest(context.Background()); err == nil {
		t.Fatal("ReadManifest traversed an ancestor symlink")
	}
}

// createDirectorySkillProject sets up a project whose skill entry points at a
// directory containing SKILL.md plus support files.
func createDirectorySkillProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	creedDir := filepath.Join(root, ".creed")
	if err := os.MkdirAll(filepath.Join(creedDir, "skills", "techgodhq", "references"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `version: 1
source:
  type: local
  path: .creed

targets:
  - name: claude
    enabled: true
    output_dir: .

skills:
  - name: techgodhq
    path: skills/techgodhq

config: []
`
	if err := os.WriteFile(filepath.Join(creedDir, "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: techgodhq\ndescription: Org-wide agent procedures.\n---\n# TechGodHQ Skill\nUse the references.\n"
	if err := os.WriteFile(filepath.Join(creedDir, "skills", "techgodhq", "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creedDir, "skills", "techgodhq", "references", "git.md"), []byte("# Git policy\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(creedDir, "skills", "techgodhq", "references", "review.md"), []byte("# Review policy\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestReadSkillDirectory(t *testing.T) {
	root := createDirectorySkillProject(t)
	source := NewSource(root)
	skill, err := source.ReadSkill(context.Background(), "techgodhq")
	if err != nil {
		t.Fatalf("ReadSkill directory skill: %v", err)
	}
	if !skill.IsDirectory() {
		t.Fatal("expected directory-shaped skill")
	}
	want := "---\nname: techgodhq\ndescription: Org-wide agent procedures.\n---\n# TechGodHQ Skill\nUse the references.\n"
	if string(skill.Content) != want {
		t.Fatalf("SKILL.md content mismatch: %q", skill.Content)
	}
	if len(skill.Files) != 2 {
		t.Fatalf("expected 2 support files, got %d: %v", len(skill.Files), skill.Files)
	}
	if string(skill.Files["references/git.md"]) != "# Git policy\n" {
		t.Fatalf("support file git.md mismatch: %q", skill.Files["references/git.md"])
	}
	if string(skill.Files["references/review.md"]) != "# Review policy\n" {
		t.Fatalf("support file review.md mismatch: %q", skill.Files["references/review.md"])
	}
}

func TestReadSkillDirectoryMissingSkillMD(t *testing.T) {
	root := createDirectorySkillProject(t)
	if err := os.Remove(filepath.Join(root, ".creed", "skills", "techgodhq", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	source := NewSource(root)
	if _, err := source.ReadSkill(context.Background(), "techgodhq"); err == nil {
		t.Fatal("expected error when directory skill lacks SKILL.md")
	}
}

func TestReadSkillDirectoryRejectsSymlink(t *testing.T) {
	root := createDirectorySkillProject(t)
	outside := filepath.Join(root, "outside.md")
	if err := os.WriteFile(outside, []byte("escaped content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".creed", "skills", "techgodhq", "references", "escape.md")); err != nil {
		t.Fatal(err)
	}
	source := NewSource(root)
	_, err := source.ReadSkill(context.Background(), "techgodhq")
	if err == nil {
		t.Fatal("expected symlink inside skill directory to be rejected")
	}
}

func TestReadSkillDirectoryOnlySkillMD(t *testing.T) {
	root := createDirectorySkillProject(t)
	if err := os.Remove(filepath.Join(root, ".creed", "skills", "techgodhq", "references", "git.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".creed", "skills", "techgodhq", "references", "review.md")); err != nil {
		t.Fatal(err)
	}
	source := NewSource(root)
	if _, err := source.ReadSkill(context.Background(), "techgodhq"); err == nil {
		t.Fatal("expected directory with only SKILL.md to be rejected in favor of a direct file entry")
	}
}
