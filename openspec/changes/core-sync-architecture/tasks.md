# Tasks — core-sync-architecture

## Goal
Implement the five capabilities from the proposal/design/specs with a testable, dependency-ordered plan.

## Task order rule
Ports and interfaces first, then adapters, then use cases/service, then generated surfaces (CLI + MCP), then wiring + integration.

## Tasks

### 1) Project foundations (dependencies / environment)
- [x] **T1: Add baseline dependencies and build tooling** *(~45m)*
  - **Files:** `go.mod`
  - **Do:** add required module deps for implementation (`gopkg.in/yaml.v3`, `github.com/go-git/go-git/v5`, `github.com/mark3labs/mcp-go`).
  - **Verify:** `go mod tidy && go test ./...` passes dependency resolution.
  - **Note:** mcp-go has no imports yet (deferred to T19+) so `go mod tidy` strips it. Will be added when Service interface + MCP surface code lands.

- [x] **T2: Add `internal/codegen` scaffolding + generator entrypoint** *(~1h)*
  - **Files:** `internal/codegen/main.go`, `internal/codegen/templates/` (as needed)
  - **Do:** create CLI entrypoint for code generation that accepts input/output paths and writes generated files.
  - **Verify:** `go run ./internal/codegen -h` shows options and returns success on `--help`.

### 2) Domain capability (`domain-model`)
- [x] **T3: Add zero-dependency domain types** *(~1h)*
  - **Files:** `internal/domain/types.go`
  - **Do:** define `Skill`, `Spec`, `ConfigFile`, `Target`, `Manifest`, `SyncResult`, `SourceConfig`, plus helper info structs (`SkillInfo`, `ConfigInfo`, `TargetInfo`) and constructors/defaults (`Manifest.Version == 1`).
  - **Verify:** `go test ./internal/domain`.

- [x] **T4: Move/refactor target registry into domain types** *(~1h)*
  - **Files:** `internal/domain/targets.go`, `internal/target` (deprecated removal or compatibility shim)
  - **Do:** centralize target metadata + emit-path definitions in one package, include all six targets with exact initial mappings.
  - **Verify:** unit test asserts claude emits both `CLAUDE.md` and `.claude/skills/`, and unknown target lookup returns error.

- [x] **T5: Add domain contract tests (no external imports)** *(~45m)*
  - **Files:** `internal/domain/types_test.go`
  - **Do:** add focused tests for defaults, target emit path behavior, and structural assertions.
  - **Verify:** `go test ./internal/domain`.

### 3) Source-reader capability (`source-reader`)
- [x] **T6: Define SourceReader port** *(~45m)*
  - **Files:** `internal/ports/source.go`
  - **Do:** define `SourceReader` interface with methods from spec.
  - **Verify:** `go test ./internal/ports` (compilation-only package check).

- [x] **T7: Implement LocalFS source adapter** *(~2h)*
  - **Files:** `internal/adapters/localfs/source.go`
  - **Do:** implement `.creed/manifest.yaml` read + list/read skill + config operations.
  - **Verify:** tests for happy path, missing manifest, malformed manifest, missing skill/config names.

- [x] **T8: Implement GitRemote source adapter (clone + cache skeleton)** *(~2h)*
  - **Files:** `internal/adapters/gitremote/source.go`, `internal/adapters/gitremote/source_test.go`
  - **Do:** clone via go-git to temp root, expose `Read*` methods by delegating to normalized local copy.
  - **Verify:** first-call clone works and methods read manifest/skills from repo path.

- [x] **T9: Add commit-cache behavior for GitRemote** *(~1h 30m)*
  - **Files:** `internal/adapters/gitremote/source.go`
  - **Do:** store/read last remote HEAD locally to short-circuit redundant clones/fetches when unchanged.
  - **Verify:** repeated read on unchanged HEAD avoids clone path (assert via test hooks / call counters).

### 4) Target-emitter capability (`target-emitter`)
- [x] **T10: Define emitter port & file DTOs** *(~45m)*
  - **Files:** `internal/ports/emitter.go`
  - **Do:** define `EmittedFile`, `EmitResult`, `TargetEmitter` with `Emit` and `Clean`.
  - **Verify:** package compiles and interface imports only standard abstractions.

- [x] **T11: Implement LocalFS emitter adapter** *(~2h)*
  - **Files:** `internal/adapters/localfs/emitter.go`
  - **Do:** implement file writes, mkdir-all behavior, per-file status (`written/skipped/error`), and clean function.
  - **Verify:** tests for nested dir creation, identical-content skip, overwrite on change, clean removes expected target files.

- [x] **T12: Migrate target registry usage to domain/ports contracts** *(~1h)*
  - **Files:** `internal/adapters/localfs/*`, `internal/usecase`, command layer callers
  - **Do:** remove direct dependency on old `internal/target` package from active flow.
  - **Verify:** `go test ./...` with `internal/target` package either fully removed from runtime or left as thin compatibility alias only.

### 5) Sync-engine capability (`sync-engine`)
- [x] **T13: Implement SyncOptions and SyncEngine orchestration** *(~1h 45m)*
  - **Files:** `internal/usecase/sync.go`
  - **Do:** add `SyncOptions{Target, DryRun, Force}` and main `Sync` flow: read manifest, resolve target set, collect files, emit through adapter, aggregate `SyncResult`.
  - **Verify:** unit test confirms enabled-target filtering and all-skill fanout.

- [x] **T14: Implement idempotent behavior and per-target reporting** *(~1h 30m)*
  - **Files:** `internal/usecase/sync.go`, tests
  - **Do:** ensure second run reports skipped when unchanged; enforce per-target summary counts and duration fields.
  - **Verify:** explicit test with temporary files + two consecutive sync runs => second run has `FilesWritten == 0`.

- [x] **T15: Handle partial failures without aborting all targets** *(~1h 15m)*
  - **Files:** `internal/usecase/sync.go`
  - **Do:** if one target emit fails, continue other targets and include error in that target result only.
  - **Verify:** fault-injection test confirms successful targets still return results.

### 6) Service-interface capability (`service-interface`)
- [x] **T16: Define Service interface and core application service implementation** *(~2h)*
  - **Files:** `internal/service/service.go`, `internal/service/impl.go`
  - **Do:** define API (`Init`, `Sync`, `AddSkill`, `RemoveSkill`, `ListSkills`, `ListTargets`, `EnableTarget`, `DisableTarget`, `Pull`, `Push`), wire dependencies.
  - **Verify:** compile-time check that all methods are implemented and return typed errors.

- [x] **T17: Implement manifest/project bootstrap (`Init`, `AddSkill`, `RemoveSkill`, list/enable/disable)** *(~2h)*
  - **Files:** `internal/service/impl.go`, `internal/usecase/*`, manifest IO helpers
  - **Do:** create `.creed/manifest.yaml` if missing, enforce defaults, and mutate manifest lists/target flags safely.
  - **Verify:** table tests around enabled/disabled target behavior + add/remove skill/config updates.

- [x] **T18: Implement sync option plumbing and pull/push behaviors** *(~1h 30m)*
  - **Files:** `internal/service/impl.go`, `internal/adapters/gitremote/source.go`
  - **Do:** map service methods to use cases; `Pull`/`Push` use git remote adapter and source config.
  - **Verify:** call-path tests validate delegation from service to engine/adapter.
  - **Note:** `Pull` delegates git remote reads through the existing GitRemote source adapter. `Push` publishes `.creed/` source changes through the system git executable until a writable git port exists.

### 7) Generated surfaces (CLI/MCP) and wiring
- [x] **T19: Add Service interface-driven code generation** *(~2h)*
  - **Files:** `internal/codegen/`, `internal/codegen/*.tmpl`
  - **Do:** implement reflection-based extraction of method names/comments/params and generate:
    - Cobra command files in `cmd/gen/*.go`
    - MCP tool files in `internal/mcp/gen/*.go`
  - **Verify:** run generator and assert generated files exist for all `Service` methods.

- [x] **T20: Add CLI generation integration + command registration** *(~1h 30m)*
  - **Files:** `cmd/root.go`, `main.go`, generated CLI files
  - **Do:** ensure generated Cobra commands are initialized and root command delegates to `Service` methods.
  - **Verify:** `go test ./cmd` and golden tests for `creed sync --target claude`, `creed init` output path and exit.

- [ ] **T21: Add MCP generation integration + server entrypoint** *(~2h)*
  - **Files:** `internal/mcp/server.go`, generated MCP files
  - **Do:** start MCP server, register tools directly from generated methods, expose structured JSON responses.
  - **Verify:** integration test enumerates tool list contains `sync`, `add_skill`, `list_targets`; calling `sync` returns structured result.

- [ ] **T22: Add generated-code generation lifecycle in CI/build** *(~45m)*
  - **Files:** `main.go`, `.github/workflows/ci.yml`, `Makefile` (if present)
  - **Do:** wire `go:generate` and CI target to run generator checks, fail on dirty tree after generation.
  - **Verify:** `go test ./...` after `go generate ./...` in clean tree.

### 8) Integration, guardrails, and cleanup
- [x] **T23: Replace old target registry/tests and remove stale stubs** *(~1h)*
  - **Files:** `internal/target` (remove or compatibility wrapper), `internal/target/target_test.go`
  - **Do:** delete/replace stale stub command behavior (`init`/`sync` no-op), align old tests to new domain contracts.
  - **Verify:** no user-visible legacy “not yet implemented” command paths remain.

- [x] **T24: Add end-to-end sync test harness** *(~2h)*
  - **Files:** `internal/usecase/sync_test.go`, `testdata/fixture-creed-project/*`
  - **Do:** full integration scenario: local `.creed` source + manifest + 2+ targets with `SyncOptions` target filter, dry-run and force modes.
  - **Verify:** one run writes, second run skips unchanged, dry-run writes no files, force rewrites identical files.

- [x] **T25: Update user docs for new behavior** *(~45m)*
  - **Files:** `README.md`, `LICENSE` checks unchanged
  - **Do:** document source model (`local`/`git`), targets, init/sync flags, and expected outputs.
  - **Verify:** docs examples match current CLI flags and manifest schema.

### 9) Delivery
- [x] **T26: Run full verification and finalize** *(~1h)*
  - **Do:** execute:
    - `go test ./...`
    - `go vet ./...`
    - `gofmt` / `golangci-lint run`
    - `go generate ./...` + `go test ./...`
  - **Verify:** all checks pass, zero failing tasks, all required artifacts exist under this change directory.