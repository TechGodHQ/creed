# creed

> One source of truth for AI context. Sync skills, specs, and config across every tool.

`creed` lets you define your AI assistant's context — skills, specifications, project
configuration — **once**, then emit it in whatever format each tool expects.

## Why?

Every AI coding tool has its own conventions:

| Tool | Context files |
|------|--------------|
| Claude Code | `CLAUDE.md`, `.claude/skills/` |
| Cursor | `.cursor/rules/` |
| Codex | `AGENTS.md` |
| Hermes Agent | `AGENTS.md`, skills directory |
| Windsurf | `.windsurfrules` |
| Aider | `.aider.conf.yml`, `CONVENTIONS.md` |

Keeping these in sync manually is fragile and tedious. `creed` is the single source —
you write your context once, and `creed` generates the right files for each target.

## Install

```bash
go install github.com/techgodhq/creed@latest
```

## Quick Start

```bash
# Initialize a creed project
creed init

# Emit context files for all configured targets
creed sync

# Emit for a specific target only
creed sync --target claude
```

## License

MIT — see [LICENSE](LICENSE).
