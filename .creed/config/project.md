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
- `internal/service/`: canonical application service used by generated CLI, MCP, and HTTP surfaces.
- `internal/codegen/`: parses `internal/service.Service` and emits operation descriptors plus generated surface glue.
- `cmd/gen/`: generated Cobra command constructors and CLI delegation.
- `internal/mcp/` and `internal/mcp/gen/`: MCP server runtime plus generated tools, schemas, and handlers.
- `internal/httpapi/` and `internal/httpapi/gen/`: generated JSON operation catalog and call routes.
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
- Generated CLI/MCP/HTTP operation surfaces sourced from `internal/service.Service` via shared operation descriptors.

Target output descriptors are the source of truth for emitted files. Each descriptor declares a path, kind, and format so the sync engine can render the right content for each target instead of guessing from bare paths. Context outputs such as `AGENTS.md`, `CLAUDE.md`, `.windsurfrules`, and Aider's `CONVENTIONS.md` receive aggregated project config. Skill directory outputs such as `.claude/skills/` and `.cursor/rules/` receive one file per skill. Target-specific config outputs use explicit renderers; Aider emits `.aider.conf.yml` pointing at `CONVENTIONS.md` plus a separate `CONVENTIONS.md` context file.

Generated interaction surfaces follow this flow: `internal/service.Service` → `go generate ./...` → operation descriptors → generated CLI commands, MCP tools, and HTTP operation routes. To add a generated operation, add a documented service method using supported input shapes (`context.Context`, no input, primitive params, or JSON-tagged DTO-like `Options`/`Request` structs), implement it on the service and fakes, run `go generate ./...`, then run `scripts/check-generated.sh` to prove generated files are current. Do not hand-wire per-operation behavior separately in CLI/MCP/HTTP; unsupported shapes should fail generation explicitly.

The HTTP surface is implemented as an operation API: `GET /v1/operations` lists the catalog and `POST /v1/operations/{operation}` invokes an operation with JSON input and a structured success/error envelope.
