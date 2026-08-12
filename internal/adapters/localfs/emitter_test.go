package localfs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

// testTarget creates a domain.Target for testing.
func testTarget() domain.Target {
	return domain.Target{
		Name:        "claude",
		DisplayName: "Claude Code",
		EmitPaths: func(projectName string) []string {
			return []string{"CLAUDE.md", ".claude/skills/"}
		},
	}
}

func TestEmitWriteNewFile(t *testing.T) {
	baseDir := t.TempDir()
	emitter := NewEmitter(baseDir)
	target := testTarget()

	files := []ports.EmittedFile{
		{Path: "CLAUDE.md", Content: []byte("# Claude Rules\nBe good.")},
	}

	results, err := emitter.Emit(context.Background(), target, files)
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != ports.EmitStatusWritten {
		t.Errorf("expected status %q, got %q", ports.EmitStatusWritten, results[0].Status)
	}

	// Verify file was written.
	content, err := os.ReadFile(filepath.Join(baseDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("failed to read emitted file: %v", err)
	}
	if string(content) != "# Claude Rules\nBe good." {
		t.Errorf("unexpected content: %q", string(content))
	}
}

func TestEmitSkipUnchanged(t *testing.T) {
	baseDir := t.TempDir()
	emitter := NewEmitter(baseDir)
	target := testTarget()

	content := []byte("# Code Review\nReview code.")
	files := []ports.EmittedFile{
		{Path: "CLAUDE.md", Content: content},
	}

	// First write.
	results, err := emitter.Emit(context.Background(), target, files)
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if results[0].Status != ports.EmitStatusWritten {
		t.Errorf("first emit: expected %q, got %q", ports.EmitStatusWritten, results[0].Status)
	}

	// Second write with identical content — should be skipped.
	results, err = emitter.Emit(context.Background(), target, files)
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if results[0].Status != ports.EmitStatusSkipped {
		t.Errorf("second emit: expected %q, got %q", ports.EmitStatusSkipped, results[0].Status)
	}
}

func TestEmitOverwriteOnChange(t *testing.T) {
	baseDir := t.TempDir()
	emitter := NewEmitter(baseDir)
	target := testTarget()

	// Write initial content.
	files := []ports.EmittedFile{
		{Path: "CLAUDE.md", Content: []byte("original content")},
	}
	if _, err := emitter.Emit(context.Background(), target, files); err != nil {
		t.Fatalf("Emit error: %v", err)
	}

	// Write changed content.
	files[0].Content = []byte("updated content")
	results, err := emitter.Emit(context.Background(), target, files)
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if results[0].Status != ports.EmitStatusWritten {
		t.Errorf("expected %q for changed content, got %q", ports.EmitStatusWritten, results[0].Status)
	}

	// Verify updated content.
	data, _ := os.ReadFile(filepath.Join(baseDir, "CLAUDE.md"))
	if string(data) != "updated content" {
		t.Errorf("expected updated content, got %q", string(data))
	}
}

func TestEmitNestedDirectory(t *testing.T) {
	baseDir := t.TempDir()
	emitter := NewEmitter(baseDir)
	target := testTarget()

	files := []ports.EmittedFile{
		{Path: ".claude/skills/code-review.md", Content: []byte("# Code Review")},
	}

	results, err := emitter.Emit(context.Background(), target, files)
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if results[0].Status != ports.EmitStatusWritten {
		t.Errorf("expected %q, got %q", ports.EmitStatusWritten, results[0].Status)
	}

	// Verify nested file exists.
	if _, err := os.Stat(filepath.Join(baseDir, ".claude", "skills", "code-review.md")); os.IsNotExist(err) {
		t.Error("nested file was not created")
	}
}

func TestEmitMultipleFiles(t *testing.T) {
	baseDir := t.TempDir()
	emitter := NewEmitter(baseDir)
	target := testTarget()

	files := []ports.EmittedFile{
		{Path: "CLAUDE.md", Content: []byte("# Claude")},
		{Path: ".claude/skills/review.md", Content: []byte("# Review")},
		{Path: ".claude/skills/testing.md", Content: []byte("# Testing")},
	}

	results, err := emitter.Emit(context.Background(), target, files)
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != ports.EmitStatusWritten {
			t.Errorf("expected %q for %s, got %q", ports.EmitStatusWritten, r.Path, r.Status)
		}
	}
}

func TestCleanRemovesTargetFiles(t *testing.T) {
	baseDir := t.TempDir()
	emitter := NewEmitter(baseDir)
	target := testTarget()

	// Emit files first.
	files := []ports.EmittedFile{
		{Path: "CLAUDE.md", Content: []byte("# Claude")},
	}
	if _, err := emitter.Emit(context.Background(), target, files); err != nil {
		t.Fatalf("Emit error: %v", err)
	}

	// Clean should remove CLAUDE.md and .claude/skills/.
	if err := emitter.Clean(context.Background(), target); err != nil {
		t.Fatalf("Clean error: %v", err)
	}

	// CLAUDE.md should be gone.
	if _, err := os.Stat(filepath.Join(baseDir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("CLAUDE.md was not removed by Clean")
	}
	// .claude/skills/ should be gone.
	if _, err := os.Stat(filepath.Join(baseDir, ".claude", "skills")); !os.IsNotExist(err) {
		t.Error(".claude/skills/ was not removed by Clean")
	}
}

func TestCleanEmptyDir(t *testing.T) {
	baseDir := t.TempDir()
	emitter := NewEmitter(baseDir)
	target := testTarget()

	// Clean on a directory with nothing emitted should not error.
	if err := emitter.Clean(context.Background(), target); err != nil {
		t.Fatalf("Clean on empty dir error: %v", err)
	}
}

func TestExistingFilesRejectsEscapingOwnershipPath(t *testing.T) {
	baseDir := t.TempDir()
	emitter := NewEmitter(baseDir)
	path := emitter.ownershipPath("claude")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal([]string{"../../outside"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := emitter.ExistingFiles(context.Background(), testTarget(), nil); err == nil {
		t.Fatal("ExistingFiles accepted an escaping ownership path")
	}
}

func TestExistingFilesIncludesCandidateWithoutOwnershipManifest(t *testing.T) {
	baseDir := t.TempDir()
	emitter := NewEmitter(baseDir)
	if err := os.MkdirAll(filepath.Join(baseDir, ".claude", "skills"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, ".claude", "skills", "review.md"), []byte("# review\n"), 0644); err != nil {
		t.Fatal(err)
	}
	files, err := emitter.ExistingFiles(context.Background(), testTarget(), []ports.EmittedFile{{Path: ".claude/skills/review.md"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != ".claude/skills/review.md" {
		t.Fatalf("candidate inventory = %#v, want matching candidate file", files)
	}
}

func TestEmitRejectsSymlinkedOwnershipDirectory(t *testing.T) {
	baseDir := t.TempDir()
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, ".creed"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(baseDir, ".creed", ".outputs")); err != nil {
		t.Fatal(err)
	}
	emitter := NewEmitter(baseDir)
	if _, err := emitter.Emit(context.Background(), testTarget(), []ports.EmittedFile{{Path: "CLAUDE.md", Content: []byte("# Claude\n")}}); err == nil {
		t.Fatal("Emit accepted a symlinked ownership directory")
	}
	if _, err := os.Stat(filepath.Join(external, "claude.json")); !os.IsNotExist(err) {
		t.Fatalf("ownership metadata escaped project: %v", err)
	}
}
