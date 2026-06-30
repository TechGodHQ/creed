package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewManifestDefaults(t *testing.T) {
	m := NewManifest()
	if m.Version != 1 {
		t.Errorf("expected Version == 1, got %d", m.Version)
	}
	if m.Source.Type != "local" {
		t.Errorf("expected Source.Type == \"local\", got %q", m.Source.Type)
	}
	if m.Source.Path != ".creed" {
		t.Errorf("expected Source.Path == \".creed\", got %q", m.Source.Path)
	}
}

func TestTargetClaudeEmitPaths(t *testing.T) {
	tg, err := LookupTarget("claude")
	if err != nil {
		t.Fatalf("LookupTarget(\"claude\") error: %v", err)
	}
	paths := tg.EmitPaths("myproject")
	if len(paths) != 2 {
		t.Fatalf("expected 2 emit paths, got %d", len(paths))
	}
	if paths[0] != "CLAUDE.md" {
		t.Errorf("expected paths[0] == \"CLAUDE.md\", got %q", paths[0])
	}
	if paths[1] != ".claude/skills/" {
		t.Errorf("expected paths[1] == \".claude/skills/\", got %q", paths[1])
	}
}

func TestLookupTargetUnknownReturnsError(t *testing.T) {
	_, err := LookupTarget("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown target, got nil")
	}
	if !errors.Is(err, ErrUnknownTarget) {
		t.Errorf("expected ErrUnknownTarget, got %v", err)
	}
}

func TestAllTargetNamesIncludesExpected(t *testing.T) {
	names := AllTargetNames()
	expected := []string{"agents", "aider", "claude", "codex", "cursor", "windsurf"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d targets, got %d: %v", len(expected), len(names), names)
	}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("expected names[%d] == %q, got %q", i, want, names[i])
		}
	}
}

func TestSyncResultZeroValue(t *testing.T) {
	var sr SyncResult
	if sr.Target != "" {
		t.Errorf("expected empty Target, got %q", sr.Target)
	}
	if sr.FilesWritten != 0 {
		t.Errorf("expected FilesWritten == 0, got %d", sr.FilesWritten)
	}
	if sr.FilesSkipped != 0 {
		t.Errorf("expected FilesSkipped == 0, got %d", sr.FilesSkipped)
	}
	if sr.Duration != 0 {
		t.Errorf("expected Duration == 0, got %v", sr.Duration)
	}
	if sr.Error != nil {
		t.Errorf("expected nil Error, got %v", sr.Error)
	}
	_ = time.Duration(0) // ensure time import is exercised
}
