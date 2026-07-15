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

func TestTargetOutputDescriptors(t *testing.T) {
	tests := []struct {
		name string
		want []TargetOutput
	}{
		{
			name: "claude",
			want: []TargetOutput{
				{Path: "CLAUDE.md", Kind: OutputKindContext, Format: "markdown"},
				{Path: ".claude/skills/", Kind: OutputKindSkillDir, Format: "markdown"},
			},
		},
		{
			name: "cursor",
			want: []TargetOutput{
				{Path: ".cursor/rules/", Kind: OutputKindSkillDir, Format: "markdown"},
			},
		},
		{
			name: "codex",
			want: []TargetOutput{
				{Path: "AGENTS.md", Kind: OutputKindContext, Format: "markdown"},
			},
		},
		{
			name: "copilot",
			want: []TargetOutput{
				{Path: ".github/copilot-instructions.md", Kind: OutputKindContext, Format: "markdown"},
			},
		},
		{
			name: "aider",
			want: []TargetOutput{
				{Path: ".aider.conf.yml", Kind: OutputKindConfig, Format: "yaml"},
				{Path: "CONVENTIONS.md", Kind: OutputKindContext, Format: "markdown"},
			},
		},
		{
			name: "gemini",
			want: []TargetOutput{
				{Path: "GEMINI.md", Kind: OutputKindContext, Format: "markdown"},
				{Path: ".gemini/", Kind: OutputKindSkillDir, Format: "markdown"},
			},
		},
		{
			name: "opencode",
			want: []TargetOutput{
				{Path: "AGENTS.md", Kind: OutputKindContext, Format: "markdown"},
				{Path: ".opencode/agents/", Kind: OutputKindSkillDir, Format: "markdown"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg, err := LookupTarget(tt.name)
			if err != nil {
				t.Fatalf("LookupTarget(%q) error: %v", tt.name, err)
			}
			outputs := tg.Outputs("myproject")
			assertTargetOutputs(t, outputs, tt.want)
		})
	}
}

func TestAllTargetsExposeOutputDescriptors(t *testing.T) {
	for _, name := range AllTargetNames() {
		t.Run(name, func(t *testing.T) {
			tg, err := LookupTarget(name)
			if err != nil {
				t.Fatalf("LookupTarget(%q) error: %v", name, err)
			}
			outputs := tg.Outputs("myproject")
			if len(outputs) == 0 {
				t.Fatalf("expected output descriptors for target %q", name)
			}
			for _, output := range outputs {
				if output.Path == "" {
					t.Fatalf("target %q has output with empty path: %#v", name, output)
				}
				if output.Kind == "" {
					t.Fatalf("target %q has output with empty kind: %#v", name, output)
				}
			}
		})
	}
}

func TestEmitPathsRemainCompatibleWithOutputDescriptors(t *testing.T) {
	for _, name := range AllTargetNames() {
		t.Run(name, func(t *testing.T) {
			tg, err := LookupTarget(name)
			if err != nil {
				t.Fatalf("LookupTarget(%q) error: %v", name, err)
			}
			outputs := tg.Outputs("myproject")
			paths := tg.EmitPaths("myproject")
			if len(paths) != len(outputs) {
				t.Fatalf("expected %d paths, got %d", len(outputs), len(paths))
			}
			for i, output := range outputs {
				if paths[i] != output.Path {
					t.Fatalf("expected paths[%d] == %q, got %q", i, output.Path, paths[i])
				}
			}
		})
	}
}

func assertTargetOutputs(t *testing.T, got, want []TargetOutput) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %d outputs, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected output[%d] == %#v, got %#v", i, want[i], got[i])
		}
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
	expected := []string{"agents", "aider", "claude", "codex", "copilot", "cursor", "gemini", "opencode", "windsurf"}
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
