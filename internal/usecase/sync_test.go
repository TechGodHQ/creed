package usecase

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techgodhq/creed/internal/adapters/localfs"
	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

// --- Mock SourceReader ---

type mockSource struct {
	manifest *domain.Manifest
	skills   map[string]*domain.Skill
	configs  map[string]*domain.ConfigFile
}

func (m *mockSource) ReadManifest(_ context.Context) (*domain.Manifest, error) {
	if m.manifest == nil {
		return nil, errors.New("manifest not found")
	}
	return m.manifest, nil
}

func (m *mockSource) ReadSkill(_ context.Context, name string) (*domain.Skill, error) {
	s, ok := m.skills[name]
	if !ok {
		return nil, errors.New("skill not found: " + name)
	}
	return s, nil
}

func (m *mockSource) ListSkills(_ context.Context) ([]domain.SkillInfo, error) {
	skills := make([]domain.SkillInfo, 0, len(m.skills))
	for _, s := range m.skills {
		skills = append(skills, domain.SkillInfo{Name: s.Name, Path: s.Path})
	}
	return skills, nil
}

func (m *mockSource) ReadConfig(_ context.Context, name string) (*domain.ConfigFile, error) {
	c, ok := m.configs[name]
	if !ok {
		return nil, errors.New("config not found: " + name)
	}
	return c, nil
}

func (m *mockSource) ListConfigs(_ context.Context) ([]domain.ConfigInfo, error) {
	configs := make([]domain.ConfigInfo, 0, len(m.configs))
	for _, c := range m.configs {
		configs = append(configs, domain.ConfigInfo{Name: c.Name, Path: c.Path})
	}
	return configs, nil
}

// --- Mock TargetEmitter (for failure injection) ---

type mockEmitter struct {
	// failOnTarget causes Emit to return an error for the named target.
	failOnTarget string
	// recorded tracks all emit calls for inspection.
	recorded []string
}

func (m *mockEmitter) Emit(_ context.Context, target domain.Target, files []ports.EmittedFile) ([]ports.EmitResult, error) {
	if target.Name == m.failOnTarget {
		return nil, errors.New("simulated emit failure for " + target.Name)
	}
	m.recorded = append(m.recorded, target.Name)
	results := make([]ports.EmitResult, 0, len(files))
	for _, f := range files {
		results = append(results, ports.EmitResult{Path: f.Path, Status: ports.EmitStatusWritten})
	}
	return results, nil
}

func (m *mockEmitter) Clean(_ context.Context, _ domain.Target) error {
	return nil
}

// --- Test Helpers ---

func newTestSource() *mockSource {
	return &mockSource{
		manifest: &domain.Manifest{
			Version: 1,
			Source:  domain.SourceConfig{Type: "local", Path: ".creed"},
			Targets: []domain.TargetConfig{
				{Name: "claude", Enabled: true},
				{Name: "cursor", Enabled: true},
				{Name: "codex", Enabled: false},
			},
			Skills: []domain.SkillEntry{
				{Name: "code-review", Path: "skills/code-review.md"},
				{Name: "testing", Path: "skills/testing.md"},
				{Name: "refactor", Path: "skills/refactor.md"},
			},
			Configs: []domain.ConfigEntry{
				{Name: "project-context", Path: "config/project-context.md"},
			},
		},
		skills: map[string]*domain.Skill{
			"code-review": {Name: "code-review", Path: "skills/code-review.md", Content: []byte("# Code Review\n\nReview code carefully.")},
			"testing":     {Name: "testing", Path: "skills/testing.md", Content: []byte("# Testing\n\nWrite tests first.")},
			"refactor":    {Name: "refactor", Path: "skills/refactor.md", Content: []byte("# Refactor\n\nKeep it clean.")},
		},
		configs: map[string]*domain.ConfigFile{
			"project-context": {Name: "project-context", Path: "config/project-context.md", Content: []byte("# Project Context\n\nThis is a creed project.")},
		},
	}
}

// --- Tests ---

func TestSync_EmitsAllSkillsToEnabledTargets(t *testing.T) {
	src := newTestSource()
	emitter := &mockEmitter{}
	engine := NewSyncEngine(src, emitter)

	result, err := engine.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should sync claude and cursor (both enabled), not codex (disabled).
	if len(result.Targets) != 2 {
		t.Fatalf("expected 2 target results, got %d", len(result.Targets))
	}

	// Claude has paths ["CLAUDE.md", ".claude/skills/"]:
	//   CLAUDE.md = 1 config file, .claude/skills/ = 3 skill files = 4 files total.
	claude := findTargetResult(t, result, "claude")
	if claude.FilesWritten != 4 {
		t.Errorf("claude: expected 4 files written, got %d", claude.FilesWritten)
	}

	// Cursor has path [".cursor/rules/"]:
	//   .cursor/rules/ = 3 skill files = 3 files total (no file paths for configs).
	cursor := findTargetResult(t, result, "cursor")
	if cursor.FilesWritten != 3 {
		t.Errorf("cursor: expected 3 files written, got %d", cursor.FilesWritten)
	}
}

func TestSync_SkipsDisabledTargets(t *testing.T) {
	src := newTestSource()
	emitter := &mockEmitter{}
	engine := NewSyncEngine(src, emitter)

	result, err := engine.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tr := range result.Targets {
		if tr.Target == "codex" {
			t.Error("disabled target 'codex' should not appear in results")
		}
	}
}

func TestSync_SpecificTargetOnly(t *testing.T) {
	src := newTestSource()
	emitter := &mockEmitter{}
	engine := NewSyncEngine(src, emitter)

	result, err := engine.Sync(context.Background(), SyncOptions{Target: "claude"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Targets) != 1 {
		t.Fatalf("expected 1 target result, got %d", len(result.Targets))
	}
	if result.Targets[0].Target != "claude" {
		t.Errorf("expected target 'claude', got %q", result.Targets[0].Target)
	}

	// Cursor should NOT have been emitted to.
	for _, name := range emitter.recorded {
		if name == "cursor" {
			t.Error("cursor should not have been emitted to when targeting claude")
		}
	}
}

func TestSync_TargetNotFoundInManifest(t *testing.T) {
	src := newTestSource()
	emitter := &mockEmitter{}
	engine := NewSyncEngine(src, emitter)

	_, err := engine.Sync(context.Background(), SyncOptions{Target: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for target not in manifest")
	}
}

func TestSync_Idempotent_SecondRunSkipsAll(t *testing.T) {
	src := newTestSource()
	tmpDir := t.TempDir()

	// Use the real LocalFS emitter to test actual skip-on-identical behavior.
	emitter := localfs.NewEmitter(tmpDir)
	engine := NewSyncEngine(src, emitter)

	// First sync: all files written.
	result1, err := engine.Sync(context.Background(), SyncOptions{Target: "claude"})
	if err != nil {
		t.Fatalf("first sync: unexpected error: %v", err)
	}
	if result1.TotalFilesWritten() == 0 {
		t.Error("first sync should have written files")
	}

	// Second sync: all files skipped (identical content).
	result2, err := engine.Sync(context.Background(), SyncOptions{Target: "claude"})
	if err != nil {
		t.Fatalf("second sync: unexpected error: %v", err)
	}

	written := result2.TotalFilesWritten()
	skipped := result2.TotalFilesSkipped()
	if written != 0 {
		t.Errorf("second sync should write 0 files, wrote %d", written)
	}
	if skipped == 0 {
		t.Error("second sync should skip files")
	}
}

func TestSync_DryRun_NoFilesWritten(t *testing.T) {
	src := newTestSource()
	tmpDir := t.TempDir()
	emitter := localfs.NewEmitter(tmpDir)
	engine := NewSyncEngine(src, emitter)

	result, err := engine.Sync(context.Background(), SyncOptions{Target: "claude", DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All files should be reported as would_write.
	claude := findTargetResult(t, result, "claude")
	for _, f := range claude.Files {
		if f.Status != StatusWouldWrite {
			t.Errorf("dry-run file %q: expected status %q, got %q", f.Path, StatusWouldWrite, f.Status)
		}
	}
	if len(claude.Files) == 0 {
		t.Error("dry-run should report files that would be written")
	}
	if claude.FilesWouldWrite != len(claude.Files) {
		t.Errorf("dry-run should count would-write files separately: got %d, want %d", claude.FilesWouldWrite, len(claude.Files))
	}
	if claude.FilesWritten != 0 {
		t.Errorf("dry-run should not count would-write files as written: got %d", claude.FilesWritten)
	}
	if result.TotalFilesWouldWrite() != len(claude.Files) {
		t.Errorf("total would-write count = %d, want %d", result.TotalFilesWouldWrite(), len(claude.Files))
	}

	// No files should exist on disk.
	if fileExists(filepath.Join(tmpDir, "CLAUDE.md")) {
		t.Error("dry-run should not write CLAUDE.md")
	}
	if fileExists(filepath.Join(tmpDir, ".claude", "skills", "code-review.md")) {
		t.Error("dry-run should not write skill files")
	}
}

func TestSync_Force_OverwritesIdenticalFiles(t *testing.T) {
	src := newTestSource()
	tmpDir := t.TempDir()
	emitter := localfs.NewEmitter(tmpDir)
	engine := NewSyncEngine(src, emitter)

	// First sync to populate files.
	_, err := engine.Sync(context.Background(), SyncOptions{Target: "claude"})
	if err != nil {
		t.Fatalf("first sync: unexpected error: %v", err)
	}

	// Second sync with Force: all files should be rewritten.
	result, err := engine.Sync(context.Background(), SyncOptions{Target: "claude", Force: true})
	if err != nil {
		t.Fatalf("force sync: unexpected error: %v", err)
	}

	claude := findTargetResult(t, result, "claude")
	if claude.FilesWritten == 0 {
		t.Error("force sync should rewrite files (FilesWritten > 0)")
	}
}

func TestSync_PartialFailure_ContinuesOtherTargets(t *testing.T) {
	src := newTestSource()
	// Mock emitter that fails on "cursor" but succeeds on "claude".
	emitter := &mockEmitter{failOnTarget: "cursor"}
	engine := NewSyncEngine(src, emitter)

	result, err := engine.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("sync should not return top-level error for partial failure: %v", err)
	}

	// Claude should succeed.
	claude := findTargetResult(t, result, "claude")
	if claude.Error != nil {
		t.Errorf("claude should not have an error: %v", claude.Error)
	}
	if claude.FilesWritten == 0 {
		t.Error("claude should have written files")
	}

	// Cursor should have an error.
	cursor := findTargetResult(t, result, "cursor")
	if cursor.Error == nil {
		t.Error("cursor should have an error")
	}
}

func TestSync_UnknownTargetInManifest(t *testing.T) {
	src := newTestSource()
	// Add an unknown target to the manifest.
	src.manifest.Targets = append(src.manifest.Targets, domain.TargetConfig{
		Name: "nonexistent-tool", Enabled: true,
	})
	emitter := &mockEmitter{}
	engine := NewSyncEngine(src, emitter)

	result, err := engine.Sync(context.Background(), SyncOptions{Target: "nonexistent-tool"})
	if err != nil {
		t.Fatalf("should not return top-level error: %v", err)
	}

	tr := result.Targets[0]
	if tr.Error == nil {
		t.Error("unknown target should produce a target-level error")
	}
}

func TestSync_EmptyManifestSkills(t *testing.T) {
	src := newTestSource()
	src.manifest.Skills = nil
	src.skills = nil

	tmpDir := t.TempDir()
	emitter := localfs.NewEmitter(tmpDir)
	engine := NewSyncEngine(src, emitter)

	// Claude: CLAUDE.md gets 1 config, .claude/skills/ gets 0 skills.
	result, err := engine.Sync(context.Background(), SyncOptions{Target: "claude"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claude := findTargetResult(t, result, "claude")
	// Only the config file should be written (no skill files).
	if claude.FilesWritten != 1 {
		t.Errorf("expected 1 file written (config only), got %d", claude.FilesWritten)
	}
}

func TestSync_EmptyManifestConfigs(t *testing.T) {
	src := newTestSource()
	src.manifest.Configs = nil
	src.configs = nil

	tmpDir := t.TempDir()
	emitter := localfs.NewEmitter(tmpDir)
	engine := NewSyncEngine(src, emitter)

	// Cursor has only directory paths — no file paths, so no config aggregation.
	// Skills still go to .cursor/rules/.
	result, err := engine.Sync(context.Background(), SyncOptions{Target: "cursor"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cursor := findTargetResult(t, result, "cursor")
	if cursor.FilesWritten != 3 {
		t.Errorf("expected 3 skill files written, got %d", cursor.FilesWritten)
	}
}

func TestSyncResult_HasErrors(t *testing.T) {
	r := &SyncResult{
		Targets: []TargetResult{
			{Target: "claude", FilesWritten: 3},
			{Target: "cursor", Error: errors.New("boom")},
		},
	}
	if !r.HasErrors() {
		t.Error("expected HasErrors to be true")
	}

	r2 := &SyncResult{
		Targets: []TargetResult{
			{Target: "claude", FilesWritten: 3},
		},
	}
	if r2.HasErrors() {
		t.Error("expected HasErrors to be false")
	}
}

func TestPrepareFiles_DirectoryGetsSkills(t *testing.T) {
	target, _ := domain.LookupTarget("cursor") // .cursor/rules/
	skills := []domain.Skill{
		{Name: "alpha", Content: []byte("alpha content")},
		{Name: "beta", Content: []byte("beta content")},
	}
	files, err := prepareFiles(target, skills, nil)
	if err != nil {
		t.Fatalf("prepare files: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	// Files should be in the skills directory.
	for _, f := range files {
		if !isDirPath(filepath.Dir(f.Path) + "/") {
			t.Errorf("file %q should be in a directory path", f.Path)
		}
	}
}

func TestPrepareFiles_FilePathGetsAggregatedConfigs(t *testing.T) {
	target, _ := domain.LookupTarget("codex") // AGENTS.md
	configs := []domain.ConfigFile{
		{Name: "ctx1", Content: []byte("context one")},
		{Name: "ctx2", Content: []byte("context two")},
	}
	files, err := prepareFiles(target, nil, configs)
	if err != nil {
		t.Fatalf("prepare files: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "AGENTS.md" {
		t.Errorf("expected AGENTS.md, got %q", files[0].Path)
	}
	expected := "context one\n---\n\ncontext two"
	if string(files[0].Content) != expected {
		t.Errorf("expected aggregated content %q, got %q", expected, string(files[0].Content))
	}
}

func TestAggregateConfigs_EmptyReturnsNil(t *testing.T) {
	if result := aggregateConfigs(nil); result != nil {
		t.Errorf("expected nil for empty configs, got %q", string(result))
	}
}

func TestPrepareFiles_UnknownOutputKindReturnsError(t *testing.T) {
	target := &domain.Target{
		Name:        "fixture",
		DisplayName: "Fixture",
		Outputs: func(string) []domain.TargetOutput {
			return []domain.TargetOutput{{Path: "fixture.out", Kind: domain.OutputKind("mystery")}}
		},
		EmitPaths: func(string) []string { return []string{"fixture.out"} },
	}

	_, err := prepareFiles(target, nil, nil)
	if err == nil {
		t.Fatal("expected unknown output kind to return an error")
	}
	if !strings.Contains(err.Error(), "unknown output kind") {
		t.Fatalf("expected unknown output kind error, got %v", err)
	}
}

func TestSync_UnknownOutputKindProducesTargetLevelError(t *testing.T) {
	const targetName = "fixture-unknown-output"
	domain.DefaultTargets[targetName] = &domain.Target{
		Name:        targetName,
		DisplayName: "Fixture Unknown Output",
		Outputs: func(string) []domain.TargetOutput {
			return []domain.TargetOutput{{Path: "fixture.out", Kind: domain.OutputKind("mystery")}}
		},
		EmitPaths: func(string) []string { return []string{"fixture.out"} },
	}
	defer delete(domain.DefaultTargets, targetName)

	src := newTestSource()
	src.manifest.Targets = []domain.TargetConfig{
		{Name: targetName, Enabled: true},
		{Name: "claude", Enabled: true},
	}
	emitter := &mockEmitter{}
	engine := NewSyncEngine(src, emitter)

	result, err := engine.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("sync should not return top-level error: %v", err)
	}
	fixture := findTargetResult(t, result, targetName)
	if fixture.Error == nil {
		t.Fatal("unknown output kind should produce a target-level error")
	}
	if !strings.Contains(fixture.Error.Error(), "unknown output kind") {
		t.Fatalf("expected unknown output kind error, got %v", fixture.Error)
	}
	claude := findTargetResult(t, result, "claude")
	if claude.Error != nil {
		t.Fatalf("other targets should continue after render failure: %v", claude.Error)
	}
	if claude.FilesWritten == 0 {
		t.Fatal("other targets should still emit files after render failure")
	}
}

func TestSync_DisabledTargetExplicitlyRequested(t *testing.T) {
	src := newTestSource()
	emitter := &mockEmitter{}
	engine := NewSyncEngine(src, emitter)

	// codex is disabled in the manifest, but an explicit Target request
	// overrides the enabled check (documented in SyncOptions.Target).
	result, err := engine.Sync(context.Background(), SyncOptions{Target: "codex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Targets) != 1 {
		t.Fatalf("expected 1 target result, got %d", len(result.Targets))
	}
	if result.Targets[0].Target != "codex" {
		t.Errorf("expected target 'codex', got %q", result.Targets[0].Target)
	}
}

func TestPrepareFiles_AiderEmitsConfigAndContext(t *testing.T) {
	target, _ := domain.LookupTarget("aider")
	configs := []domain.ConfigFile{
		{Name: "project", Content: []byte("project context")},
		{Name: "development", Content: []byte("development rules")},
	}
	files, err := prepareFiles(target, nil, configs)
	if err != nil {
		t.Fatalf("prepare files: %v", err)
	}
	byPath := emittedFilesByPath(files)

	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if got := string(byPath[".aider.conf.yml"].Content); got != "read:\n  - CONVENTIONS.md\n" {
		t.Errorf("unexpected aider config content: %q", got)
	}
	expectedContext := "project context\n---\n\ndevelopment rules"
	if got := string(byPath["CONVENTIONS.md"].Content); got != expectedContext {
		t.Errorf("expected CONVENTIONS.md content %q, got %q", expectedContext, got)
	}
}

func TestPrepareFiles_DescriptorAwareCandidateFiles(t *testing.T) {
	skills := []domain.Skill{
		{Name: "review", Content: []byte("review skill")},
	}
	configs := []domain.ConfigFile{
		{Name: "project", Content: []byte("project context")},
	}
	tests := []struct {
		name  string
		paths []string
	}{
		{name: "claude", paths: []string{"CLAUDE.md", ".claude/skills/review.md"}},
		{name: "codex", paths: []string{"AGENTS.md"}},
		{name: "cursor", paths: []string{".cursor/rules/review.md"}},
		{name: "aider", paths: []string{".aider.conf.yml", "CONVENTIONS.md"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := domain.LookupTarget(tt.name)
			if err != nil {
				t.Fatalf("lookup target: %v", err)
			}
			files, err := prepareFiles(target, skills, configs)
			if err != nil {
				t.Fatalf("prepare files: %v", err)
			}
			paths := emittedFilePaths(files)
			assertStringSlicesEqual(t, paths, tt.paths)
		})
	}
}

func TestPrepareFiles_DeterministicOrdering(t *testing.T) {
	target, _ := domain.LookupTarget("claude")
	skills := []domain.Skill{
		{Name: "review", Content: []byte("review skill")},
		{Name: "testing", Content: []byte("testing skill")},
	}
	configs := []domain.ConfigFile{
		{Name: "project", Content: []byte("project context")},
	}

	firstFiles, err := prepareFiles(target, skills, configs)
	if err != nil {
		t.Fatalf("prepare files: %v", err)
	}
	first := emittedFilePaths(firstFiles)
	for range 5 {
		nextFiles, err := prepareFiles(target, skills, configs)
		if err != nil {
			t.Fatalf("prepare files: %v", err)
		}
		next := emittedFilePaths(nextFiles)
		assertStringSlicesEqual(t, next, first)
	}
}

func TestSync_DescriptorOrderMatchesEmittedPathOrder(t *testing.T) {
	tests := []struct {
		name      string
		outputDir string
	}{
		{name: "agents"},
		{name: "aider"},
		{name: "claude"},
		{name: "codex"},
		{name: "cursor", outputDir: "generated"},
		{name: "windsurf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := newTestSource()
			src.manifest.Targets = []domain.TargetConfig{{
				Name:      tt.name,
				Enabled:   true,
				OutputDir: tt.outputDir,
			}}
			tmpDir := t.TempDir()
			engine := NewSyncEngine(src, localfs.NewEmitter(tmpDir))

			first, err := engine.Sync(context.Background(), SyncOptions{})
			if err != nil {
				t.Fatalf("first sync: %v", err)
			}
			firstResult := findTargetResult(t, first, tt.name)
			if firstResult.Error != nil {
				t.Fatalf("first sync target error: %v", firstResult.Error)
			}

			target, err := domain.LookupTarget(tt.name)
			if err != nil {
				t.Fatalf("lookup target: %v", err)
			}
			target = targetWithOutputDir(target, normalizeTargetConfig(src.manifest.Targets[0]).OutputDir)
			expected := expandedDescriptorPaths(target.Outputs(""), src.manifest.Skills)
			assertStringSlicesEqual(t, emittedResultPaths(firstResult.Files), expected)

			second, err := engine.Sync(context.Background(), SyncOptions{})
			if err != nil {
				t.Fatalf("second sync: %v", err)
			}
			secondResult := findTargetResult(t, second, tt.name)
			if secondResult.Error != nil {
				t.Fatalf("second sync target error: %v", secondResult.Error)
			}
			assertStringSlicesEqual(t, emittedResultPaths(secondResult.Files), expected)
			if secondResult.FilesWritten != 0 {
				t.Fatalf("second sync should be idempotent and write 0 files, wrote %d", secondResult.FilesWritten)
			}
			if secondResult.FilesSkipped != len(expected) {
				t.Fatalf("second sync should skip %d files, skipped %d", len(expected), secondResult.FilesSkipped)
			}
		})
	}
}

func TestSync_AiderWithOutputDirEmitsConfigAndContext(t *testing.T) {
	src := newTestSource()
	src.manifest.Targets = []domain.TargetConfig{{Name: "aider", Enabled: true, OutputDir: "generated"}}
	tmpDir := t.TempDir()
	engine := NewSyncEngine(src, localfs.NewEmitter(tmpDir))

	result, err := engine.Sync(context.Background(), SyncOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	aider := findTargetResult(t, result, "aider")
	if aider.FilesWritten != 2 {
		t.Fatalf("expected 2 files written, got %d: %#v", aider.FilesWritten, aider.Files)
	}
	assertStringSlicesEqual(t, emittedResultPaths(aider.Files), []string{
		"generated/.aider.conf.yml",
		"generated/CONVENTIONS.md",
	})
	if !fileExists(filepath.Join(tmpDir, "generated", ".aider.conf.yml")) {
		t.Fatal("expected generated/.aider.conf.yml to be written")
	}
	if !fileExists(filepath.Join(tmpDir, "generated", "CONVENTIONS.md")) {
		t.Fatal("expected generated/CONVENTIONS.md to be written")
	}
	configContent, err := os.ReadFile(filepath.Join(tmpDir, "generated", ".aider.conf.yml"))
	if err != nil {
		t.Fatalf("read generated aider config: %v", err)
	}
	if !strings.Contains(string(configContent), "generated/CONVENTIONS.md") {
		t.Fatalf("expected aider config to reference generated conventions file, got %q", string(configContent))
	}
}

func TestPrepareFiles_AiderWithoutConfigsDoesNotEmitDanglingConfig(t *testing.T) {
	target, _ := domain.LookupTarget("aider")
	tests := []struct {
		name    string
		configs []domain.ConfigFile
	}{
		{name: "no configs"},
		{name: "empty config content", configs: []domain.ConfigFile{{Name: "empty"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := prepareFiles(target, nil, tt.configs)
			if err != nil {
				t.Fatalf("prepare files: %v", err)
			}

			if len(files) != 0 {
				t.Fatalf("expected no files without config content, got %#v", files)
			}
		})
	}
}

func TestPrepareFiles_AiderConfigReferencesDescriptorContextPath(t *testing.T) {
	target := &domain.Target{
		Name:        "aider",
		DisplayName: "Aider",
		Outputs: func(string) []domain.TargetOutput {
			return []domain.TargetOutput{
				{Path: "docs/CONTEXT.md", Kind: domain.OutputKindContext, Format: "markdown"},
				{Path: ".aider.conf.yml", Kind: domain.OutputKindConfig, Format: "yaml"},
			}
		},
		EmitPaths: func(string) []string { return []string{".aider.conf.yml", "docs/CONTEXT.md"} },
	}
	configs := []domain.ConfigFile{
		{Name: "project", Content: []byte("project context")},
		{Name: "rules", Content: []byte("development rules")},
	}

	files, err := prepareFiles(target, nil, configs)
	if err != nil {
		t.Fatalf("prepare files: %v", err)
	}
	byPath := emittedFilesByPath(files)

	if got := string(byPath[".aider.conf.yml"].Content); got != "read:\n  - docs/CONTEXT.md\n" {
		t.Fatalf("aider config should reference descriptor context path, got %q", got)
	}
	if got := string(byPath["docs/CONTEXT.md"].Content); got != "project context\n---\n\ndevelopment rules" {
		t.Fatalf("context descriptor should receive aggregated configs, got %q", got)
	}
}

// --- Helpers ---

func emittedFilesByPath(files []ports.EmittedFile) map[string]ports.EmittedFile {
	byPath := make(map[string]ports.EmittedFile, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	return byPath
}

func emittedFilePaths(files []ports.EmittedFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func emittedResultPaths(files []FileResult) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func expandedDescriptorPaths(outputs []domain.TargetOutput, skills []domain.SkillEntry) []string {
	paths := make([]string, 0, len(outputs)+len(skills))
	for _, output := range outputs {
		if output.Kind == domain.OutputKindSkillDir {
			for _, skill := range skills {
				paths = append(paths, output.Path+skill.Name+".md")
			}
			continue
		}
		paths = append(paths, output.Path)
	}
	return paths
}

func assertStringSlicesEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d paths %v, got %d paths %v", len(want), want, len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("path %d: expected %q, got %q (all paths: %v)", i, want[i], got[i], got)
		}
	}
}

func findTargetResult(t *testing.T, result *SyncResult, name string) TargetResult {
	t.Helper()
	for _, tr := range result.Targets {
		if tr.Target == name {
			return tr
		}
	}
	t.Fatalf("target result %q not found", name)
	return TargetResult{}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
