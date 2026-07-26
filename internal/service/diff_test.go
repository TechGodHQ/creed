package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/techgodhq/creed/internal/usecase"
)

func TestDiffReportsCleanChangedNewAndDeletedOutput(t *testing.T) {
	root := t.TempDir()
	svc := New(root)
	ctx := context.Background()
	if err := svc.Init(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	writeProjectConfig(t, root)
	if _, err := svc.Sync(ctx, usecase.SyncOptions{Target: "codex"}); err != nil {
		t.Fatal(err)
	}

	clean, err := svc.Diff(ctx, usecase.DiffOptions{Target: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if clean.HasDifferences() || clean.UnifiedDiff() != "" {
		t.Fatalf("clean diff = %#v, want no differences", clean)
	}

	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	changed, err := svc.Diff(ctx, usecase.DiffOptions{Target: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if !changed.HasDifferences() || !strings.Contains(changed.UnifiedDiff(), "--- a/AGENTS.md") || !strings.Contains(changed.UnifiedDiff(), "+# Project") {
		t.Fatalf("changed diff = %q, want unified AGENTS.md diff", changed.UnifiedDiff())
	}
	if !strings.Contains(changed.UnifiedDiff(), "@@ -1,1 +1,") {
		t.Fatalf("changed diff uses invalid replacement range: %q", changed.UnifiedDiff())
	}

	if err := os.Remove(agents); err != nil {
		t.Fatal(err)
	}
	created, err := svc.Diff(ctx, usecase.DiffOptions{Target: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(created.UnifiedDiff(), "--- /dev/null\n+++ b/AGENTS.md") {
		t.Fatalf("created diff = %q", created.UnifiedDiff())
	}

	if _, err := svc.Sync(ctx, usecase.SyncOptions{Target: "cursor"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveSkill(ctx, "review"); err != nil {
		t.Fatal(err)
	}
	// A normal sync does not delete obsolete outputs; diff must continue to
	// recognize the formerly generated path after refreshing the inventory.
	if _, err := svc.Sync(ctx, usecase.SyncOptions{Target: "cursor"}); err != nil {
		t.Fatal(err)
	}
	deleted, err := svc.Diff(ctx, usecase.DiffOptions{Target: "cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deleted.UnifiedDiff(), "--- a/.cursor/rules/review.md\n+++ /dev/null") {
		t.Fatalf("deleted diff = %q", deleted.UnifiedDiff())
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "rules", "user.md"), []byte("keep\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(deleted.UnifiedDiff(), "user.md") {
		t.Fatalf("diff adopted unrelated user output: %q", deleted.UnifiedDiff())
	}
}
