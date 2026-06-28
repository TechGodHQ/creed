package target

import (
	"testing"
)

func TestRegistry(t *testing.T) {
	expected := []string{"claude", "cursor", "codex", "agents", "windsurf", "aider"}
	for _, name := range expected {
		if _, ok := Registry[name]; !ok {
			t.Errorf("expected target %q in Registry", name)
		}
	}
}

func TestClaudeEmitPaths(t *testing.T) {
	tg := Registry["claude"]
	if tg == nil {
		t.Fatal("claude target not found")
	}
	paths := tg.EmitPaths("myproject")
	if len(paths) != 2 {
		t.Fatalf("expected 2 emit paths, got %d", len(paths))
	}
	if paths[0] != "CLAUDE.md" {
		t.Errorf("expected CLAUDE.md, got %s", paths[0])
	}
}

func TestAllTargets(t *testing.T) {
	names := AllTargets()
	if len(names) != len(Registry) {
		t.Errorf("AllTargets() returned %d, Registry has %d", len(names), len(Registry))
	}
}
