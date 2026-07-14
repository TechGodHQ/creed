package integration

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/techgodhq/creed/internal/service"
	"github.com/techgodhq/creed/internal/usecase"
)

func TestDogfoodFixtureSyncsAllAgentTargetsAndIsIdempotent(t *testing.T) {
	root := copyDogfoodFixture(t)
	svc := service.New(root)

	first, err := svc.Sync(context.Background(), usecase.SyncOptions{})
	if err != nil {
		t.Fatalf("first dogfood sync: %v", err)
	}
	if first.HasErrors() {
		t.Fatalf("first dogfood sync returned target errors: %#v", first.Targets)
	}
	if first.TotalFilesWritten() == 0 {
		t.Fatal("first dogfood sync should write generated files")
	}

	assertFileContains(t, filepath.Join(root, "CLAUDE.md"), "# Dogfood Project")
	assertFileContains(t, filepath.Join(root, "CLAUDE.md"), "# Dogfood Development")
	assertFileContains(t, filepath.Join(root, ".claude", "skills", "review.md"), "# Review")
	assertFileContains(t, filepath.Join(root, ".claude", "skills", "testing.md"), "# Testing")
	assertFileContains(t, filepath.Join(root, "AGENTS.md"), "# Dogfood Project")
	assertFileContains(t, filepath.Join(root, ".cursor", "rules", "review.md"), "# Review")
	assertFileContains(t, filepath.Join(root, ".cursor", "rules", "testing.md"), "# Testing")
	assertFileContains(t, filepath.Join(root, "GEMINI.md"), "# Dogfood Project")
	assertFileContains(t, filepath.Join(root, ".gemini", "review.md"), "# Review")
	assertFileContains(t, filepath.Join(root, ".gemini", "testing.md"), "# Testing")
	assertFileContains(t, filepath.Join(root, ".aider.conf.yml"), "CONVENTIONS.md")
	assertFileContains(t, filepath.Join(root, "CONVENTIONS.md"), "# Dogfood Project")

	conventionsBefore := readFileString(t, filepath.Join(root, "CONVENTIONS.md"))
	second, err := svc.Sync(context.Background(), usecase.SyncOptions{})
	if err != nil {
		t.Fatalf("second dogfood sync: %v", err)
	}
	if second.HasErrors() {
		t.Fatalf("second dogfood sync returned target errors: %#v", second.Targets)
	}
	if second.TotalFilesWritten() != 0 {
		t.Fatalf("second dogfood sync wrote %d files, want 0", second.TotalFilesWritten())
	}
	if second.TotalFilesSkipped() == 0 {
		t.Fatal("second dogfood sync should skip unchanged files")
	}
	if got := readFileString(t, filepath.Join(root, "CONVENTIONS.md")); got != conventionsBefore {
		t.Fatalf("CONVENTIONS.md changed after idempotent sync\nbefore:\n%s\nafter:\n%s", conventionsBefore, got)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func copyDogfoodFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Join("testdata", "dogfood-creed")
	dst := t.TempDir()
	if err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		dstPath := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, 0644)
	}); err != nil {
		t.Fatalf("copy dogfood fixture: %v", err)
	}
	return dst
}
