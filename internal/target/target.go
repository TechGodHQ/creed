package target

// Target represents an AI tool that can receive synced context.
type Target struct {
	Name        string // canonical name (e.g., "claude", "cursor")
	DisplayName string // human-readable name
	// EmitPaths returns the relative file paths this target expects.
	// e.g., ["CLAUDE.md", ".claude/skills/"]
	EmitPaths func(projectName string) []string
}

// Registry maps target names to their definitions.
var Registry = map[string]*Target{
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

// AllTargets returns the names of all registered targets.
func AllTargets() []string {
	names := make([]string, 0, len(Registry))
	for name := range Registry {
		names = append(names, name)
	}
	return names
}
