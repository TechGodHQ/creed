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
