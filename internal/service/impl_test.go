package service

import (
	"context"
	"fmt"
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
	for _, name := range []string{"agents", "aider", "copilot", "gemini", "windsurf"} {
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
		{
			name: "copilot",
			want: []domain.TargetOutput{
				{Path: ".github/copilot-instructions.md", Kind: domain.OutputKindContext, Format: "markdown"},
			},
		},
		{
			name: "gemini",
			want: []domain.TargetOutput{
				{Path: "GEMINI.md", Kind: domain.OutputKindContext, Format: "markdown"},
				{Path: ".gemini/", Kind: domain.OutputKindSkillDir, Format: "markdown"},
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

func TestAddRemoveConfigMutatesManifest(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := svc.AddConfig(ctx, "testing", "config/testing.md"); err != nil {
		t.Fatalf("AddConfig() error = %v", err)
	}
	configs, err := svc.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}
	if len(configs) != 3 || configs[0].Name != "development" || configs[1].Name != "project" || configs[2].Name != "testing" || configs[2].Path != "config/testing.md" {
		t.Fatalf("ListConfigs() = %#v, want alphabetically ordered configs including testing", configs)
	}
	if err := svc.AddConfig(ctx, "testing", "config/testing-v2.md"); err != nil {
		t.Fatalf("AddConfig(update) error = %v", err)
	}
	configs, err = svc.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs(update) error = %v", err)
	}
	if len(configs) != 3 || configs[2].Path != "config/testing-v2.md" {
		t.Fatalf("updated configs = %#v, want one updated entry", configs)
	}
	if err := svc.RemoveConfig(ctx, "testing"); err != nil {
		t.Fatalf("RemoveConfig() error = %v", err)
	}
	configs, err = svc.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs(after remove) error = %v", err)
	}
	if len(configs) != 2 || configs[0].Name != "development" || configs[1].Name != "project" {
		t.Fatalf("configs after remove = %#v, want alphabetically ordered default configs", configs)
	}
}

func TestConfigOperationsValidateNamesAndMissingEntries(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := svc.AddConfig(ctx, "", ""); err == nil || !strings.Contains(err.Error(), "config name is required") {
		t.Fatalf("AddConfig(empty name) error = %v, want config name error", err)
	}
	if err := svc.RemoveConfig(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "config not found: missing") {
		t.Fatalf("RemoveConfig(missing) error = %v, want missing config error", err)
	}
	if err := svc.AddConfig(ctx, "default", ""); err != nil {
		t.Fatalf("AddConfig(default path) error = %v", err)
	}
	configs, err := svc.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}
	for _, config := range configs {
		if config.Name == "default" && config.Path != "config/default.md" {
			t.Fatalf("default config path = %q, want config/default.md", config.Path)
		}
	}
	if len(configs) != 3 || configs[0].Name != "default" || configs[1].Name != "development" || configs[2].Name != "project" {
		t.Fatalf("configs are not sorted after add: %#v", configs)
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

func TestValidateReportsManifestAndSourceDiagnostics(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".creed", "config", "empty.md"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	manifest := `version: 1
source:
  type: local
  path: .creed
targets:
  - name: nope
    enabled: true
    output_dir: .
skills:
  - name: duplicate
    path: skills/review.md
  - name: duplicate
    path: ../escape.md
config:
  - name: duplicate
    path: config/empty.md
  - name: other
    path: skills/review.md
`
	if err := os.WriteFile(filepath.Join(root, ".creed", "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Validate(ctx)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Valid {
		t.Fatalf("Validate() result = %#v, want invalid", result)
	}
	for _, code := range []string{"unknown_target", "duplicate_source_name", "unsafe_source_path", "duplicate_source_path"} {
		if !hasDiagnostic(result.Errors, code) {
			t.Fatalf("Validate() errors = %#v, missing %q", result.Errors, code)
		}
	}
	if !hasDiagnostic(result.Warnings, "empty_source_content") {
		t.Fatalf("Validate() warnings = %#v, missing empty-source warning", result.Warnings)
	}
}

func TestValidateAcceptsScaffoldedProject(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	result, err := svc.Validate(ctx)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !result.Valid || len(result.Errors) != 0 {
		t.Fatalf("Validate() = %#v, want valid result", result)
	}
}

func TestValidateRejectsUnknownSchemaFieldsAndMissingVersion(t *testing.T) {
	for name, manifest := range map[string]string{
		"unknown field": `version: 1
source:
  type: local
  unexpected: value
targets: []
unknown: value
`,
		"missing version": `source:
  type: local
targets: []
`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			creedDir := filepath.Join(root, ".creed")
			if err := os.MkdirAll(creedDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(creedDir, "manifest.yaml"), []byte(manifest), 0644); err != nil {
				t.Fatal(err)
			}
			result, err := New(root).Validate(context.Background())
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if result.Valid || len(result.Errors) == 0 {
				t.Fatalf("Validate() = %#v, want schema error", result)
			}
		})
	}
}

func hasDiagnostic(diagnostics []ValidationDiagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
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

func TestGlobalDiscoveryDoesNotRequireManifest(t *testing.T) {
	svc := New(t.TempDir())
	ctx := context.Background()

	targets, err := svc.ListTargets(ctx)
	if err != nil {
		t.Fatalf("ListTargets() error = %v", err)
	}
	if len(targets) != len(domain.AllTargetNames()) {
		t.Fatalf("ListTargets() returned %d targets, want %d", len(targets), len(domain.AllTargetNames()))
	}
	for _, target := range targets {
		if target.Enabled || target.OutputDir != "" {
			t.Fatalf("unconfigured target = %#v, want disabled with no output directory", target)
		}
	}

	skills, err := svc.ListSkills(ctx)
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("ListSkills() = %#v, want no registered skills", skills)
	}
}

func TestDoctorHealthyProject(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()

	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.Sync(ctx, usecase.SyncOptions{}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	report, err := svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}

	if !report.ManifestOK {
		t.Errorf("ManifestOK = false, want true")
	}
	if !report.SourceDirOK {
		t.Errorf("SourceDirOK = false, want true")
	}
	if !report.Validation.Valid {
		t.Errorf("Validation.Valid = false, want true; errors = %#v", report.Validation.Errors)
	}
	if report.HasErrors() {
		t.Errorf("HasErrors() = true, want false; checks = %#v", report.Checks)
	}
	if !filepath.IsAbs(report.Root) {
		t.Errorf("Root = %q, want absolute path", report.Root)
	}
	if len(report.Targets) == 0 {
		t.Errorf("Targets is empty, want configured targets")
	}

	enabled := map[string]bool{}
	for _, target := range report.Targets {
		enabled[target.Name] = target.Enabled
	}
	for _, name := range []string{"claude", "codex", "cursor"} {
		if !enabled[name] {
			t.Errorf("target %s should be enabled by default", name)
		}
	}

	if report.GitAvailable && report.GitPath == "" {
		t.Errorf("GitAvailable = true but GitPath is empty")
	}
}

func TestDoctorReportsOutputDrift(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sync(ctx, usecase.SyncOptions{Target: "codex"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("clobbered\n"), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if !report.Drifted || !report.HasErrors() {
		t.Fatalf("Doctor() drift = %#v, want an error-level drift report", report)
	}
	if !hasDoctorCheck(report.Checks, "error", "output_drift") {
		t.Fatalf("Doctor() checks = %#v, want output_drift", report.Checks)
	}
}

func TestDoctorMissingCreedDir(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()

	report, err := svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}

	if report.SourceDirOK {
		t.Errorf("SourceDirOK = true, want false for missing .creed")
	}
	if report.ManifestOK {
		t.Errorf("ManifestOK = true, want false for missing manifest")
	}
	if !report.HasErrors() {
		t.Errorf("HasErrors() = false, want true for missing setup; checks = %#v", report.Checks)
	}

	if !hasDoctorCheck(report.Checks, "error", "missing_source_dir") {
		t.Errorf("missing_source_dir error not found; checks = %#v", report.Checks)
	}
	if !hasDoctorCheck(report.Checks, "error", "missing_manifest") {
		t.Errorf("missing_manifest error not found; checks = %#v", report.Checks)
	}
}

func TestDoctorInvalidManifestReportsValidationErrors(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()

	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	badManifest := `version: 1
source:
  type: local
  path: .creed
targets:
  - name: nonexistent
    enabled: true
    output_dir: .
skills:
  - name: review
    path: skills/review.md
config:
  - name: project
    path: config/project.md
`
	if err := os.WriteFile(filepath.Join(root, ".creed", "manifest.yaml"), []byte(badManifest), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}

	if report.Validation.Valid {
		t.Errorf("Validation.Valid = true, want false for bad manifest")
	}
	if !report.HasErrors() {
		t.Errorf("HasErrors() = false, want true")
	}
}

func TestDoctorNeverExposesSensitiveRemote(t *testing.T) {
	root := t.TempDir()
	svc := New(root, WithGitToken("secret-token-do-not-leak"))
	ctx := context.Background()

	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Configure git source with a remote containing credentials in the URL.
	remote := "https://ci-user:hunter2@git.example.com/repo.git"
	manifest := fmt.Sprintf(`version: 1
source:
  type: git
  path: .creed
  remote: %s
targets:
  - name: claude
    enabled: true
    output_dir: .
skills:
  - name: review
    path: skills/review.md
config:
  - name: project
    path: config/project.md
`, remote)
	if err := os.WriteFile(filepath.Join(root, ".creed", "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	report, err := svc.Doctor(ctx)
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}

	// The token passed via WithGitToken must never appear anywhere.
	serialized := fmt.Sprintf("%#v", report)
	if strings.Contains(serialized, "secret-token-do-not-leak") {
		t.Errorf("report serialization leaked git token")
	}

	// The password embedded in the remote URL must be redacted.
	if strings.Contains(report.SourceRemote, "hunter2") {
		t.Errorf("SourceRemote leaked URL password: %s", report.SourceRemote)
	}
	// URL userinfo must be removed completely; usernames may be tokens.
	if strings.Contains(report.SourceRemote, "ci-user") {
		t.Errorf("SourceRemote leaked URL username: %s", report.SourceRemote)
	}
	// The host and path should be intact.
	if !strings.Contains(report.SourceRemote, "git.example.com/repo.git") {
		t.Errorf("SourceRemote should preserve host/path; got: %s", report.SourceRemote)
	}

	for _, check := range report.Checks {
		if strings.Contains(check.Message, "secret-token-do-not-leak") || strings.Contains(check.Detail, "secret-token-do-not-leak") {
			t.Errorf("check leaked git token: %+v", check)
		}
		if strings.Contains(check.Message, "hunter2") || strings.Contains(check.Detail, "hunter2") {
			t.Errorf("check leaked URL password: %+v", check)
		}
	}
}

func TestDoctorCreedPathIsFileNotDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".creed"), []byte("not a dir"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := New(root)

	report, err := svc.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor() error = %v", err)
	}
	if report.SourceDirOK {
		t.Errorf("SourceDirOK = true, want false when .creed is not a directory")
	}
	if !hasDoctorCheck(report.Checks, "error", "source_not_directory") {
		t.Errorf("source_not_directory error not found; checks = %#v", report.Checks)
	}
}

func hasDoctorCheck(checks []DoctorCheck, kind, code string) bool {
	for _, c := range checks {
		if c.Kind == kind && c.Code == code {
			return true
		}
	}
	return false
}

func writeSkillProject(t *testing.T, manifest string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	creedDir := filepath.Join(root, ".creed")
	if err := os.MkdirAll(creedDir, 0755); err != nil {
		t.Fatal(err)
	}
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(creedDir, "manifest.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestValidateAcceptsDirectorySkillWithFrontmatter(t *testing.T) {
	root := writeSkillProject(t, `version: 1
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
`, map[string]string{
		".creed/skills/techgodhq/SKILL.md":          "---\nname: techgodhq\ndescription: Org procedures.\n---\n# Org\n",
		".creed/skills/techgodhq/references/git.md": "# Git\n",
		".creed/skills/techgodhq/templates/pr.md":   "# PR\n",
	})
	result, err := New(root).Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !result.Valid {
		t.Fatalf("Validate() errors = %#v", result.Errors)
	}
}

func TestValidateDirectorySkillDiagnostics(t *testing.T) {
	base := `version: 1
source:
  type: local
  path: .creed
targets:
  - name: claude
    enabled: true
    output_dir: .
skills:
  - name: %s
    path: skills/%s
config: []
`
	cases := []struct {
		name       string
		skillName  string
		dirName    string
		skillMD    string
		wantCode   string
		wantIn     string // "errors" or "warnings"
		extraFiles map[string]string
	}{
		{
			name:      "missing SKILL.md",
			skillName: "techgodhq", dirName: "techgodhq",
			skillMD: "", wantCode: "missing_source_file", wantIn: "errors",
		},
		{
			name:      "frontmatter name mismatch",
			skillName: "techgodhq", dirName: "techgodhq",
			skillMD:  "---\nname: other\ndescription: x\n---\n# S\n",
			wantCode: "skill_name_mismatch", wantIn: "errors",
		},
		{
			name:      "missing description",
			skillName: "techgodhq", dirName: "techgodhq",
			skillMD:  "---\nname: techgodhq\n---\n# S\n",
			wantCode: "missing_skill_description", wantIn: "errors",
		},
		{
			name:      "no frontmatter warns",
			skillName: "techgodhq", dirName: "techgodhq",
			skillMD:  "# Plain skill\n",
			wantCode: "missing_skill_frontmatter", wantIn: "warnings",
		},
		{
			name:      "unterminated frontmatter",
			skillName: "techgodhq", dirName: "techgodhq",
			skillMD:  "---\nname: techgodhq\n# never closed\n",
			wantCode: "unterminated_skill_frontmatter", wantIn: "errors",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{}
			if tc.skillMD != "" {
				files[".creed/skills/"+tc.dirName+"/SKILL.md"] = tc.skillMD
			}
			for p, c := range tc.extraFiles {
				files[p] = c
			}
			root := writeSkillProject(t, fmt.Sprintf(base, tc.skillName, tc.dirName), files)
			result, err := New(root).Validate(context.Background())
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			var diags []ValidationDiagnostic
			if tc.wantIn == "errors" {
				diags = result.Errors
			} else {
				diags = result.Warnings
			}
			if !hasDiagnostic(diags, tc.wantCode) {
				t.Fatalf("Validate() %s = %#v, missing %q", tc.wantIn, diags, tc.wantCode)
			}
		})
	}
}

func TestValidateDirectorySkillRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	creedDir := filepath.Join(root, ".creed")
	skillDir := filepath.Join(creedDir, "skills", "techgodhq")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: techgodhq\ndescription: x\n---\n# S\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(skillDir, "escape.md")); err != nil {
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
	result, err := New(root).Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if result.Valid {
		t.Fatalf("Validate() = valid, want symlink rejection; errors=%#v", result.Errors)
	}
	if !hasDiagnostic(result.Errors, "symlink_source_file") {
		t.Fatalf("Validate() errors = %#v, missing symlink_source_file", result.Errors)
	}
}

func TestValidateWarnsWhenSkillsHaveNoOutputTarget(t *testing.T) {
	root := writeSkillProject(t, `version: 1
source:
  type: local
  path: .creed
targets:
  - name: agents
    enabled: true
    output_dir: .
skills:
  - name: plain
    path: skills/plain.md
config: []
`, map[string]string{
		".creed/skills/plain.md": "---\nname: plain\ndescription: A skill.\n---\n# Plain\n",
	})
	result, err := New(root).Validate(context.Background())
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !hasDiagnostic(result.Warnings, "skills_have_no_output") {
		t.Fatalf("Validate() warnings = %#v, missing skills_have_no_output", result.Warnings)
	}
	if !result.Valid {
		t.Fatalf("Validate() errors = %#v, want valid (warning only)", result.Errors)
	}
}

func TestSyncDirectorySkillEndToEndNoDataLoss(t *testing.T) {
	root := writeSkillProject(t, `version: 1
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
`, map[string]string{
		".creed/skills/techgodhq/SKILL.md":                 "---\nname: techgodhq\ndescription: Org procedures.\n---\n# Org Skill\nBody.\n",
		".creed/skills/techgodhq/references/git.md":        "# Git policy\nSigned commits required.\n",
		".creed/skills/techgodhq/references/review.md":     "# Review policy\nTwo reviewers.\n",
		".creed/skills/techgodhq/templates/pr-template.md": "# PR\nTemplate body.\n",
		".creed/skills/techgodhq/scripts/check.sh":         "#!/bin/sh\nexit 0\n",
	})
	svc := New(root)
	ctx := context.Background()

	validateResult, err := svc.Validate(ctx)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !validateResult.Valid {
		t.Fatalf("Validate() errors = %#v", validateResult.Errors)
	}

	syncResult, err := svc.Sync(ctx, usecase.SyncOptions{Target: "claude"})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if syncResult.HasErrors() {
		t.Fatalf("Sync() target errors: %#v", syncResult.Targets)
	}
	if syncResult.TotalFilesWritten() != 5 {
		t.Fatalf("TotalFilesWritten() = %d, want 5; result=%#v", syncResult.TotalFilesWritten(), syncResult)
	}

	emitted := map[string]string{
		".claude/skills/techgodhq/SKILL.md":                 "---\nname: techgodhq\ndescription: Org procedures.\n---\n# Org Skill\nBody.\n",
		".claude/skills/techgodhq/references/git.md":        "# Git policy\nSigned commits required.\n",
		".claude/skills/techgodhq/references/review.md":     "# Review policy\nTwo reviewers.\n",
		".claude/skills/techgodhq/templates/pr-template.md": "# PR\nTemplate body.\n",
		".claude/skills/techgodhq/scripts/check.sh":         "#!/bin/sh\nexit 0\n",
	}
	for path, want := range emitted {
		got := mustRead(t, filepath.Join(root, filepath.FromSlash(path)))
		if got != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}

	// Zero data loss: every source file under the skill directory appears
	// under the emitted skill directory, and vice versa.
	sourceFiles := 0
	err = filepath.Walk(filepath.Join(root, ".creed", "skills", "techgodhq"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			sourceFiles++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	emittedFiles := 0
	err = filepath.Walk(filepath.Join(root, ".claude", "skills", "techgodhq"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			emittedFiles++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if sourceFiles != emittedFiles || sourceFiles != 5 {
		t.Fatalf("file count mismatch: source=%d emitted=%d, want 5/5", sourceFiles, emittedFiles)
	}

	// Second sync is a no-op.
	second, err := svc.Sync(ctx, usecase.SyncOptions{Target: "claude"})
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if second.TotalFilesWritten() != 0 {
		t.Fatalf("second sync wrote %d files, want 0 (idempotent)", second.TotalFilesWritten())
	}
	if second.TotalFilesSkipped() != 5 {
		t.Fatalf("second sync skipped %d files, want 5", second.TotalFilesSkipped())
	}
}

func TestValidateFlatSkillFrontmatterDiagnostics(t *testing.T) {
	base := `version: 1
source:
  type: local
  path: .creed
targets:
  - name: claude
    enabled: true
    output_dir: .
skills:
  - name: demo
    path: skills/%s
config: []
`
	cases := []struct {
		name     string
		file     string
		content  string
		wantCode string
		wantIn   string
	}{
		{"mismatch", "demo.md", "---\nname: other\ndescription: x\n---\n# S\n", "skill_name_mismatch", "errors"},
		{"missing description", "demo.md", "---\nname: demo\n---\n# S\n", "missing_skill_description", "errors"},
		{"no frontmatter", "demo.md", "# Plain\n", "missing_skill_frontmatter", "warnings"},
		{"valid", "demo.md", "---\nname: demo\ndescription: A skill.\n---\n# S\n", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeSkillProject(t, fmt.Sprintf(base, tc.file), map[string]string{
				".creed/skills/" + tc.file: tc.content,
			})
			result, err := New(root).Validate(context.Background())
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tc.wantCode == "" {
				if !result.Valid {
					t.Fatalf("Validate() errors = %#v, want valid", result.Errors)
				}
				return
			}
			var diags []ValidationDiagnostic
			if tc.wantIn == "errors" {
				diags = result.Errors
			} else {
				diags = result.Warnings
			}
			if !hasDiagnostic(diags, tc.wantCode) {
				t.Fatalf("Validate() %s = %#v, missing %q", tc.wantIn, diags, tc.wantCode)
			}
		})
	}
}
