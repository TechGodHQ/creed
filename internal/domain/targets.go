package domain

import (
	"errors"
	"sort"
)

// ErrUnknownTarget is returned when a target name is not found in the registry.
var ErrUnknownTarget = errors.New("unknown target")

// DefaultTargets is the canonical registry of all supported targets.
// Each target maps a canonical name to its metadata and semantic output logic.
var DefaultTargets = map[string]*Target{
	"claude": newTarget("claude", "Claude Code", []TargetOutput{
		{Path: "CLAUDE.md", Kind: OutputKindContext, Format: "markdown"},
		{Path: ".claude/skills/", Kind: OutputKindSkillDir, Format: "markdown"},
	}),
	"cursor": newTarget("cursor", "Cursor", []TargetOutput{
		{Path: ".cursor/rules/", Kind: OutputKindSkillDir, Format: "markdown"},
	}),
	"codex": newTarget("codex", "OpenAI Codex", []TargetOutput{
		{Path: "AGENTS.md", Kind: OutputKindContext, Format: "markdown"},
	}),
	"copilot": newTarget("copilot", "GitHub Copilot", []TargetOutput{
		{Path: ".github/copilot-instructions.md", Kind: OutputKindContext, Format: "markdown"},
	}),
	"gemini": newTarget("gemini", "Gemini CLI", []TargetOutput{
		{Path: "GEMINI.md", Kind: OutputKindContext, Format: "markdown"},
		{Path: ".gemini/", Kind: OutputKindSkillDir, Format: "markdown"},
	}),
	"opencode": newTarget("opencode", "OpenCode", []TargetOutput{
		{Path: "AGENTS.md", Kind: OutputKindContext, Format: "markdown"},
		{Path: ".opencode/agents/", Kind: OutputKindSkillDir, Format: "markdown"},
	}),
	"agents": newTarget("agents", "AGENTS.md (generic)", []TargetOutput{
		{Path: "AGENTS.md", Kind: OutputKindContext, Format: "markdown"},
	}),
	"windsurf": newTarget("windsurf", "Windsurf", []TargetOutput{
		{Path: ".windsurfrules", Kind: OutputKindContext, Format: "text"},
	}),
	"aider": newTarget("aider", "Aider", []TargetOutput{
		{Path: ".aider.conf.yml", Kind: OutputKindConfig, Format: "yaml"},
		{Path: "CONVENTIONS.md", Kind: OutputKindContext, Format: "markdown"},
	}),
}

func newTarget(name, displayName string, outputs []TargetOutput) *Target {
	copiedOutputs := cloneOutputs(outputs)
	target := &Target{
		Name:        name,
		DisplayName: displayName,
	}
	target.Outputs = func(projectName string) []TargetOutput {
		return cloneOutputs(copiedOutputs)
	}
	target.EmitPaths = func(projectName string) []string {
		return emitPathsFromOutputs(copiedOutputs)
	}
	return target
}

func cloneOutputs(outputs []TargetOutput) []TargetOutput {
	cloned := make([]TargetOutput, len(outputs))
	copy(cloned, outputs)
	return cloned
}

func emitPathsFromOutputs(outputs []TargetOutput) []string {
	paths := make([]string, 0, len(outputs))
	for _, output := range outputs {
		paths = append(paths, output.Path)
	}
	return paths
}

// LookupTarget returns the target definition for the given name.
// Returns ErrUnknownTarget if the name is not registered.
func LookupTarget(name string) (*Target, error) {
	t, ok := DefaultTargets[name]
	if !ok {
		return nil, ErrUnknownTarget
	}
	return t, nil
}

// AllTargetNames returns the sorted names of all registered targets.
func AllTargetNames() []string {
	names := make([]string, 0, len(DefaultTargets))
	for name := range DefaultTargets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
