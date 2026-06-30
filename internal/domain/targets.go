package domain

import (
	"errors"
	"sort"
)

// ErrUnknownTarget is returned when a target name is not found in the registry.
var ErrUnknownTarget = errors.New("unknown target")

// DefaultTargets is the canonical registry of all supported targets.
// Each target maps a canonical name to its metadata and emit-path logic.
var DefaultTargets = map[string]*Target{
	"claude": {
		Name:        "claude",
		DisplayName: "Claude Code",
		EmitPaths: func(projectName string) []string {
			return []string{"CLAUDE.md", ".claude/skills/"}
		},
	},
	"cursor": {
		Name:        "cursor",
		DisplayName: "Cursor",
		EmitPaths: func(projectName string) []string {
			return []string{".cursor/rules/"}
		},
	},
	"codex": {
		Name:        "codex",
		DisplayName: "OpenAI Codex",
		EmitPaths: func(projectName string) []string {
			return []string{"AGENTS.md"}
		},
	},
	"agents": {
		Name:        "agents",
		DisplayName: "AGENTS.md (generic)",
		EmitPaths: func(projectName string) []string {
			return []string{"AGENTS.md"}
		},
	},
	"windsurf": {
		Name:        "windsurf",
		DisplayName: "Windsurf",
		EmitPaths: func(projectName string) []string {
			return []string{".windsurfrules"}
		},
	},
	"aider": {
		Name:        "aider",
		DisplayName: "Aider",
		EmitPaths: func(projectName string) []string {
			return []string{".aider.conf.yml", "CONVENTIONS.md"}
		},
	},
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
