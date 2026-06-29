## Why

Creed needs a foundational architecture that supports its core mission: syncing AI context (skills, specs, config) from a single source to multiple target tools. This first change establishes the domain model, port/adapter structure, sync engine, and code-generated service surface that all future features build on. Getting this right from day one prevents structural debt as we add targets, backends, and interaction surfaces (CLI, MCP server, HTTP API).

## What Changes

- Define core domain types: `Skill`, `Spec`, `ConfigFile`, `Target`, `Manifest`, `SyncResult`
- Implement `SourceReader` port with two adapters: `LocalFS` (read from `.creed/` in repo) and `GitRemote` (clone/pull from a git URL)
- Implement `TargetEmitter` port with a `LocalFS` adapter (write files to target-specific paths)
- Build `SyncEngine` use case: read source → resolve targets → emit files → report results. Idempotent by design
- Establish `Service` interface as the single API surface, with code generation producing thin wrappers for CLI (Cobra) and MCP server. HTTP wrapper deferred but the interface guarantees compatibility
- Define target registry for: claude, cursor, codex, agents (generic AGENTS.md), windsurf, aider
- Support declarative `creed.yaml` config describing which targets to emit and where the source lives
- Support file-level sync only — skills/config treated as opaque blobs, no content transformation

## Non-goals

- **No content transformation or parsing** — creed moves files, doesn't rewrite markdown between target formats. Scoped for a future change.
- **No HTTP server component in this change** — the service interface is designed to support it, but only CLI + MCP are implemented now.
- **No hosted/team features** — no user accounts, no web UI, no team registries. Git remote covers the sync use case.
- **No drift detection heuristics** — sync overwrites. Reconciliation logic is a future enhancement.
- **No skill versioning or dependency resolution** — files are synced as-is from source.

## Capabilities

### New Capabilities
- `domain-model`: Core types and interfaces that define the vocabulary of the system
- `source-reader`: Port for reading canonical context from local filesystem or git remote
- `target-emitter`: Port for writing context files to tool-specific paths
- `sync-engine`: Use case orchestrating source → target emission with idempotent semantics
- `service-interface`: Generated API surface shared across CLI, MCP, and future HTTP server

### Modified Capabilities
<!-- None — this is the initial architecture. -->

## Impact

- **New packages**: `internal/domain`, `internal/ports`, `internal/adapters/localfs`, `internal/adapters/gitremote`, `internal/usecase`, `internal/service`
- **CLI**: Rewrite current stub commands (`init`, `sync`) to delegate to service interface
- **MCP**: New `internal/mcp` package with tool registrations generated from service interface
- **Dependencies**: `go-git` (git remote support), `mark3labs/mcp-go` or equivalent (MCP server), existing `spf13/cobra`
- **Config**: `creed.yaml` format defined and validated
- **Existing code**: Current `cmd/` stubs and `internal/target/` registry will be refactored into the new architecture
