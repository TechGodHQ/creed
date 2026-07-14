# Creed Architecture

Creed syncs AI-agent context from one canonical source directory into the file
layouts expected by multiple coding tools. The architecture is intentionally
hexagonal: the sync policy is isolated from filesystem and git details.

## Layers

```text
cmd/, internal/mcp/, internal/httpapi       generated interaction surfaces
          |
generated operation descriptors             shared surface contract
          |
internal/service                            canonical application API
          |
internal/usecase                            sync orchestration
          |
internal/ports                              source/emitter interfaces
          |
internal/domain                             manifest, targets, result vocabulary
          |
internal/adapters                           local filesystem and git implementations
```

## Canonical API

`internal/service.Service` is the contract every user-facing surface should wrap:

- `Init` bootstraps `.creed/manifest.yaml`.
- `Sync` emits configured source files into enabled targets.
- `AddSkill` / `RemoveSkill` mutate manifest skill entries.
- `ListSkills` / `ListTargets` expose manifest and registry state.
- `EnableTarget` / `DisableTarget` mutate manifest target state.
- `Pull` / `Push` support git-backed source sharing.

CLI, MCP, and HTTP all wrap this service through generated operation descriptors.
The generated code owns per-surface names, request decoding, schema/catalog data,
handler delegation, and structured success/error envelopes so behavior does not
drift between surfaces.

## Generated interaction surfaces

`go generate ./...` runs `internal/codegen`, which parses
`internal/service.Service` and writes generated files under:

- `cmd/gen/` for Cobra command constructors and command metadata.
- `internal/mcp/gen/` for MCP tool definitions, schemas, request decoding, and
  service delegation.
- `internal/httpapi/gen/` for the HTTP operation catalog and JSON call routing.
- shared generated operation descriptor data consumed by those surfaces.

The source-of-truth flow is:

```text
internal/service.Service
        ↓ parser + metadata resolver
operation descriptors
        ↓
CLI renderer     MCP renderer     HTTP renderer
        ↓              ↓              ↓
cmd/gen       internal/mcp/gen   internal/httpapi/gen
```

Generated surfaces preserve the existing public operation names while removing
handwritten per-operation switch logic from each surface. Runtime packages may
still provide small adapters and transport setup, but operation-specific decoding
and service calls belong in generated code.

The HTTP surface is intentionally operation-oriented rather than hand-designed
REST for this phase:

- `GET /v1/operations` returns the generated operation catalog.
- `POST /v1/operations/{operation}` calls one operation with a JSON input body.
- Responses use the same structured success/error envelope model as MCP.

### Adding a generated operation

1. Add a documented method to `internal/service.Service`.
2. Keep inputs generator-supported: `context.Context`, no input, primitive params,
   or a DTO-like `Options`/`Request` struct whose exported fields have JSON tags.
3. Implement the method on the concrete service and on fake services used by tests.
4. Run `go generate ./...` to refresh generated CLI, MCP, HTTP, and descriptor files.
5. Add or update tests that prove the service behavior and any surface-specific
   contract that matters for the operation.
6. Run `scripts/check-generated.sh` before handoff; it fails if generation is not
   idempotent or produces uncommitted generated output.

Unsupported method shapes fail generation explicitly. Do not add silent skips or
surface-local hand wiring for new operations; that recreates the drift the
generator is meant to eliminate.

## Source readers

A source reader implements `internal/ports.SourceReader`:

```go
ReadManifest(ctx) (*domain.Manifest, error)
ReadSkill(ctx, name) (*domain.Skill, error)
ListSkills(ctx) ([]domain.SkillInfo, error)
ReadConfig(ctx, name) (*domain.ConfigFile, error)
ListConfigs(ctx) ([]domain.ConfigInfo, error)
```

Implemented adapters:

- `localfs.Source`: reads `.creed/` in the current project.
- `gitremote.Source`: clones or reuses a cached git repository, then delegates
  reads to the local filesystem adapter.

## Target emitters

A target emitter implements `internal/ports.TargetEmitter`:

```go
Emit(ctx, target, files) ([]ports.EmitResult, error)
Clean(ctx, target) error
```

`localfs.Emitter` writes files under the project root. It creates parent
directories, writes atomically, preserves normal 0644 source-file permissions,
and skips files whose content is already identical.

## Sync flow

`internal/usecase.SyncEngine` performs one sync run:

1. Read `.creed/manifest.yaml`.
2. Resolve either a requested target (`--target`) or all enabled targets.
3. Validate `output_dir` so emitted paths cannot escape the project root.
4. Read all manifest-declared skills and config files.
5. For each target, prepare emitted files from target path metadata.
6. Emit files, collecting per-file and per-target result data.

Partial target failures are isolated: one failed target does not prevent the next
target from running. A top-level error is reserved for failures that prevent the
sync from being planned at all, such as an unreadable manifest.

## Target output semantics

Targets define structured output descriptors in `internal/domain/targets.go`. The
legacy `EmitPaths` view is still available for compatibility, but descriptors are
the canonical model for rendering and inspection.

Each descriptor declares a path, output kind, and format:

- Context outputs receive concatenated manifest config content, such as
  `AGENTS.md`, `CLAUDE.md`, `.windsurfrules`, and Aider's `CONVENTIONS.md`.
- Skill directory outputs receive one file per manifest skill, such as
  `.claude/skills/` and `.cursor/rules/`.
- Target-specific config outputs are rendered by explicit target renderers, such
  as Aider's `.aider.conf.yml` pointing at `CONVENTIONS.md`.

The service list-target DTO exposes descriptors alongside legacy emit paths so
CLI, MCP, HTTP, and any future generated surfaces can inspect target behavior
without re-deriving it from filenames.

## Guardrails

- `output_dir` must be relative and cannot contain `..` traversal.
- Atomic writes use temp-file plus rename.
- Temp files are chmodded to `0644` before rename.
- `--dry-run` reports which files would change and which files are already
  identical without writing.
- `--force` cleans the target's emitted paths before writing.
- Git remote cache metadata guards against redundant clones when remote HEAD is
  unchanged.

## Tests

Unit tests cover each layer in isolation. `internal/integration` covers the
end-to-end behavior from fixture `.creed/` sources through real emitters,
including local-source sync, git-remote-source sync, dry-run, and idempotent
re-sync.
