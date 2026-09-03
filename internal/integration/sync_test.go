package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techgodhq/creed/internal/adapters/gitremote"
	"github.com/techgodhq/creed/internal/adapters/localfs"
	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
)

func TestServiceSyncEndToEndLocalSource(t *testing.T) {
	root := newFixtureProject(t)

	result, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{Target: "claude"})
	if err != nil {
		t.Fatalf("sync claude: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("sync returned target errors: %#v", result.Targets)
	}
	claude := onlyTarget(t, result, "claude")
	if claude.FilesWritten != 3 {
		t.Fatalf("claude files written = %d, want 3", claude.FilesWritten)
	}

	assertFileContains(t, filepath.Join(root, "CLAUDE.md"), "# Project Context")
	assertFileContains(t, filepath.Join(root, ".claude", "skills", "review.md"), "# Review")
	assertFileContains(t, filepath.Join(root, ".claude", "skills", "testing.md"), "# Testing")
}

func TestServiceSyncEndToEndAllTargetsAndIdempotent(t *testing.T) {
	root := newFixtureProject(t)

	first, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{})
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if first.HasErrors() {
		t.Fatalf("first sync returned target errors: %#v", first.Targets)
	}
	if first.TotalFilesWritten() == 0 {
		t.Fatal("first sync should write target files")
	}
	assertAllEnabledTargetFiles(t, root)

	second, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{})
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if second.TotalFilesWritten() != 0 {
		t.Fatalf("second sync wrote %d files, want 0", second.TotalFilesWritten())
	}
	if second.TotalFilesSkipped() == 0 {
		t.Fatal("second sync should skip unchanged files")
	}
}

func TestServiceSyncEndToEndDryRunReportsDiffWithoutWriting(t *testing.T) {
	root := newFixtureProject(t)

	fresh, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{Target: "claude", DryRun: true})
	if err != nil {
		t.Fatalf("fresh dry-run sync: %v", err)
	}
	freshClaude := onlyTarget(t, fresh, "claude")
	if len(freshClaude.Files) != 4 {
		t.Fatalf("fresh dry-run reported %d files, want 4", len(freshClaude.Files))
	}
	for _, file := range freshClaude.Files {
		if file.Status != usecase.StatusWouldWrite {
			t.Fatalf("fresh dry-run file %s status = %s, want %s", file.Path, file.Status, usecase.StatusWouldWrite)
		}
	}
	assertFileMissing(t, filepath.Join(root, "CLAUDE.md"))
	assertFileMissing(t, filepath.Join(root, ".claude", "skills", "review.md"))

	if _, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{Target: "claude"}); err != nil {
		t.Fatalf("write before idempotent dry-run: %v", err)
	}
	idempotent, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{Target: "claude", DryRun: true})
	if err != nil {
		t.Fatalf("idempotent dry-run sync: %v", err)
	}
	idempotentClaude := onlyTarget(t, idempotent, "claude")
	if idempotentClaude.FilesSkipped != 4 {
		t.Fatalf("idempotent dry-run skipped = %d, want 4", idempotentClaude.FilesSkipped)
	}
	for _, file := range idempotentClaude.Files {
		if file.Status != usecase.StatusSkipped {
			t.Fatalf("idempotent dry-run file %s status = %s, want %s", file.Path, file.Status, usecase.StatusSkipped)
		}
	}
}

func TestServiceSyncEndToEndForceRewritesIdenticalFiles(t *testing.T) {
	root := newFixtureProject(t)

	if _, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{Target: "claude"}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	forced, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{Target: "claude", Force: true})
	if err != nil {
		t.Fatalf("force sync: %v", err)
	}
	claude := onlyTarget(t, forced, "claude")
	if claude.FilesWritten != 3 {
		t.Fatalf("force sync files written = %d, want 3", claude.FilesWritten)
	}
}

func TestGitRemoteSourceEndToEndSyncAllTargets(t *testing.T) {
	remoteWorktree := newFixtureProject(t)
	initGitRepo(t, remoteWorktree)

	root := t.TempDir()
	source := gitremote.NewSource(remoteWorktree, "")
	defer source.Cleanup()
	engine := usecase.NewSyncEngine(source, localfs.NewEmitter(root))

	result, err := engine.Sync(context.Background(), usecase.SyncOptions{})
	if err != nil {
		t.Fatalf("git remote sync: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("git remote sync returned target errors: %#v", result.Targets)
	}
	assertAllEnabledTargetFiles(t, root)
}

func newFixtureProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, ".creed/manifest.yaml", `version: 1
source:
  type: local
  path: .creed
targets:
  - name: claude
    enabled: true
    output_dir: .
  - name: cursor
    enabled: true
    output_dir: .
  - name: codex
    enabled: true
    output_dir: .
  - name: agents
    enabled: true
    output_dir: .
  - name: windsurf
    enabled: true
    output_dir: .
  - name: aider
    enabled: true
    output_dir: .
skills:
  - name: review
    path: skills/review.md
  - name: testing
    path: skills/testing.md
config:
  - name: project
    path: config/project.md
`)
	writeFile(t, root, ".creed/skills/review.md", "# Review\n\nReview the diff against the task goal.\n")
	writeFile(t, root, ".creed/skills/testing.md", "# Testing\n\nRun the smallest meaningful test first.\n")
	writeFile(t, root, ".creed/config/project.md", "# Project Context\n\nThis context is shared across tools.\n")
	return root
}

func TestGeneratedAttributesPreserveUserRulesAndAreIdempotent(t *testing.T) {
	root := newFixtureProject(t)
	writeFile(t, root, ".gitattributes", "*.lock binary\n")

	first, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{Target: "claude"})
	if err != nil || first.HasErrors() {
		t.Fatalf("first sync: result=%#v err=%v", first, err)
	}
	attributes := filepath.Join(root, ".gitattributes")
	assertFileContains(t, attributes, "*.lock binary")
	assertFileContains(t, attributes, "# creed:generated claude begin")
	assertFileContains(t, attributes, "CLAUDE.md linguist-generated=true")
	assertFileContains(t, attributes, ".claude/skills/review.md linguist-generated=true")

	second, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{Target: "claude"})
	if err != nil || second.HasErrors() || second.TotalFilesWritten() != 0 {
		t.Fatalf("idempotent sync: result=%#v err=%v", second, err)
	}
	diff, err := service.New(root).Diff(context.Background(), usecase.DiffOptions{Target: "claude"})
	if err != nil || diff.HasDifferences() {
		t.Fatalf("sync/diff disagreement: diff=%#v err=%v", diff, err)
	}

	// A CRLF checkout must not duplicate or rewrite Creed-owned blocks.
	contents, err := os.ReadFile(attributes)
	if err != nil {
		t.Fatalf("read attributes for CRLF test: %v", err)
	}
	if err := os.WriteFile(attributes, []byte(strings.ReplaceAll(string(contents), "\n", "\r\n")), 0644); err != nil {
		t.Fatalf("write CRLF attributes: %v", err)
	}
	crlf, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{Target: "claude"})
	if err != nil || crlf.HasErrors() || crlf.TotalFilesWritten() != 0 {
		t.Fatalf("CRLF idempotent sync: result=%#v err=%v", crlf, err)
	}

	// Disabled targets retain Creed-owned attribution blocks, matching the
	// stale-output policy: Creed reports stale output rather than deleting
	// metadata that may now coexist with user-authored rules.
	manifest := filepath.Join(root, ".creed", "manifest.yaml")
	manifestContents, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	disabled := strings.Replace(string(manifestContents), "name: claude\n    enabled: true", "name: claude\n    enabled: false", 1)
	if disabled == string(manifestContents) {
		t.Fatal("fixture did not disable claude target")
	}
	if err := os.WriteFile(manifest, []byte(disabled), 0644); err != nil {
		t.Fatalf("disable claude target: %v", err)
	}
	if _, err := service.New(root).Sync(context.Background(), usecase.SyncOptions{}); err != nil {
		t.Fatalf("sync after disabling target: %v", err)
	}
	assertFileContains(t, attributes, "# creed:generated claude begin")
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertAllEnabledTargetFiles(t *testing.T, root string) {
	t.Helper()
	assertFileContains(t, filepath.Join(root, "CLAUDE.md"), "# Project Context")
	assertFileContains(t, filepath.Join(root, ".claude", "skills", "review.md"), "# Review")
	assertFileContains(t, filepath.Join(root, ".claude", "skills", "testing.md"), "# Testing")
	assertFileContains(t, filepath.Join(root, ".cursor", "rules", "review.md"), "# Review")
	assertFileContains(t, filepath.Join(root, ".cursor", "rules", "testing.md"), "# Testing")
	assertFileContains(t, filepath.Join(root, "AGENTS.md"), "# Project Context")
	assertFileContains(t, filepath.Join(root, ".windsurfrules"), "# Project Context")
	assertFileContains(t, filepath.Join(root, ".aider.conf.yml"), "CONVENTIONS.md")
	assertFileContains(t, filepath.Join(root, "CONVENTIONS.md"), "# Project Context")
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s does not contain %q; got:\n%s", path, want, string(data))
	}
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists or stat returned non-ENOENT error: %v", path, err)
	}
}

func onlyTarget(t *testing.T, result *usecase.SyncResult, name string) usecase.TargetResult {
	t.Helper()
	for _, target := range result.Targets {
		if target.Target == name {
			return target
		}
	}
	t.Fatalf("target %q not found in result: %#v", name, result.Targets)
	return usecase.TargetResult{}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.name", "Creed Test")
	runGit(t, dir, "config", "user.email", "creed-test@example.invalid")
	runGit(t, dir, "add", ".creed")
	runGit(t, dir, "commit", "-m", "add creed fixture")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}
