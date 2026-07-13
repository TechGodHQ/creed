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
- Descriptor-aware target output rendering for context files, skill directories, and target-specific config files.

Target output descriptors are the source of truth for emitted files. Each descriptor declares a path, kind, and format so the sync engine can render the right content for each target instead of guessing from bare paths. Context outputs such as `AGENTS.md`, `CLAUDE.md`, `.windsurfrules`, and Aider's `CONVENTIONS.md` receive aggregated project config. Skill directory outputs such as `.claude/skills/` and `.cursor/rules/` receive one file per skill. Target-specific config outputs use explicit renderers; Aider emits `.aider.conf.yml` pointing at `CONVENTIONS.md` plus a separate `CONVENTIONS.md` context file.

---

# Development Instructions

## Commands

Run these before handing work back:

```bash
go build ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l .
```

If code generation is touched, also run:

```bash
go generate ./...
go test -race -count=1 ./...
```

## Style

- Keep package boundaries clean: domain types must not import adapters or use cases.
- Ports live in `internal/ports`; adapters implement ports without leaking filesystem/git details into use cases.
- Prefer small interfaces and explicit DTOs over maply-typed blobs.
- Public exported Go identifiers need doc comments.
- Tests should cover real behavior, not just compile-time existence.
- Preserve deterministic output ordering for generated/synced files.

## Git / PR Rules

- Commits should use Shiv's global git identity so GitHub verification works.
- Runner-generated commits may include `Co-authored-by: Archon <archon@purelymail.com>` for attribution.
- Do not merge PRs automatically; human review/merge is required.

## OpenSpec

OpenSpec CLI is not installed on this machine. Edit files directly under `openspec/changes/<change>/` when creating or updating specs.

For meaningful changes:

1. Add or update `proposal.md`, `design.md`, `tasks.md`, and `specs/**/spec.md`.
2. Keep implementation PRs small and dependency-ordered.
3. Update `tasks.md` as implementation lands.
4. Do not rewrite an approved proposal during execution; log deviations elsewhere.
