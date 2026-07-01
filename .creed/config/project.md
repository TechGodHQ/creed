# Creed Project Context

Creed is a Go CLI that syncs AI agent context files across coding tools. The canonical source lives in `.creed/`; running `creed sync` emits tool-specific files such as `AGENTS.md`, `CLAUDE.md`, and `.cursor/rules/*`.

## Purpose

One source of truth for AI coding context. Define project instructions, skills, and conventions once, then emit the layout each agent tool expects.

## Repository

- GitHub: `github.com/techgodhq/creed`
- Module: `github.com/techgodhq/creed`
- Language: Go
- CLI framework: Cobra
- Architecture: ports-and-adapters / hexagonal

## Important Directories

- `cmd/`: Cobra CLI commands.
- `internal/domain/`: zero-dependency domain types and target registry.
- `internal/ports/`: source-reader and target-emitter interfaces.
- `internal/adapters/localfs/`: local filesystem source and emitter.
- `internal/adapters/gitremote/`: git-backed source reader.
- `internal/usecase/`: sync orchestration and result model.
- `internal/service/`: canonical application service used by CLI/MCP/future surfaces.
- `internal/mcp/`: MCP server surface.
- `internal/codegen/`: code-generation scaffold.
- `openspec/changes/`: spec-driven change artifacts.
- `.creed/`: dogfooded source context for this repo.

## Current Product Shape

Creed currently supports:

- `.creed/manifest.yaml` as the source manifest.
- Local source reads from `.creed/`.
- Git remote source reads via go-git clone/cache.
- Enabled target syncing to local filesystem output dirs.
- Targets: `claude`, `cursor`, `codex`, `agents`, `windsurf`, `aider`.
- Dry-run, force, idempotent writes, and structured sync results.

Known limitation: target-specific file semantics are still crude. The sync engine aggregates config files into the first file output and emits skills into directory outputs. Targets with multiple non-directory files, such as Aider, need richer mapping semantics.
