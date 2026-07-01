# Creed Architecture

Creed syncs AI-agent context from one canonical source directory into the file
layouts expected by multiple coding tools. The architecture is intentionally
hexagonal: the sync policy is isolated from filesystem and git details.

## Layers

```text
cmd/ and internal/mcp/       interaction surfaces
          |
internal/service             canonical application API
          |
internal/usecase             sync orchestration
          |
internal/ports               source/emitter interfaces
          |
internal/domain              manifest, targets, result vocabulary
          |
internal/adapters            local filesystem and git implementations
```

## Canonical API

`internal/service.Service` is the contract every user-facing surface should wrap:

- `Init` bootstraps `.creed/manifest.yaml`.
- `Sync` emits configured source files into enabled targets.
- `AddSkill` / `RemoveSkill` mutate manifest skill entries.
- `ListSkills` / `ListTargets` expose manifest and registry state.
- `EnableTarget` / `DisableTarget` mutate manifest target state.
- `Pull` / `Push` support git-backed source sharing.

The CLI currently delegates directly to this service. MCP and future HTTP layers
should remain thin wrappers around the same interface so behavior does not drift.

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

## Target path semantics

Targets define `EmitPaths` in `internal/domain/targets.go`.

- Directory paths ending in `/` receive one file per skill.
- The first file path receives concatenated config file content.
- Extra file paths are intentionally ignored until Creed grows per-path content
  semantics. This keeps the current sync model file-level and predictable.

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
