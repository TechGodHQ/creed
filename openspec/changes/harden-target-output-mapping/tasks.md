# Tasks — harden-target-output-mapping

## Goal

Make target-output-mapping a durable, inspectable product surface: explicit renderers, structured target output inspection, Aider-safe semantics, dogfood regression, and updated docs/context.

## Task order rule

Renderer contract first, then structured listing surfaces and regressions in parallel, then docs/context cleanup, then final verification/archive.

## Tasks

### 1) Renderer contract and sync internals

- [x] **T1: Extract explicit target output renderer contract** *(~1h 30m)* — Linear: COD-326
  - **Files:** `internal/usecase/sync.go`, focused tests as needed
  - **Do:** replace ad-hoc output-kind branching with renderer helpers/registry for context, skill directory, config, and target-specific config outputs.
  - **Verify:** existing sync/usecase tests still pass; unknown output kind returns a structured target-level error instead of silently skipping.

- [x] **T2: Harden Aider renderer semantics** *(~1h)* — Linear: COD-327
  - **Files:** `internal/usecase/sync.go`, `internal/usecase/*_test.go` or integration tests
  - **Do:** make `.aider.conf.yml` rendering deterministic and explicitly tied to the context descriptor path; ensure `CONVENTIONS.md` receives aggregated context/config content.
  - **Verify:** tests assert exact `.aider.conf.yml` content and `CONVENTIONS.md` content for a manifest with multiple config entries.
  - **Depends on:** T1

### 2) Inspectable target descriptors

- [x] **T3: Expose output descriptors through service target listing** *(~1h 30m)* — Linear: COD-328
  - **Files:** `internal/domain/types.go`, `internal/service/impl.go`, service tests
  - **Do:** extend target listing DTOs to include structured output descriptors while preserving `EmitPaths` compatibility.
  - **Verify:** service tests assert every target includes descriptor path/kind/format and legacy emit paths remain present.

- [x] **T4: Surface descriptors in CLI/MCP list-target behavior** *(~1h 30m)* — Linear: COD-329
  - **Files:** generated CLI/MCP command behavior, `internal/codegen` if needed, command/MCP tests
  - **Do:** ensure list-target surfaces expose descriptor details in a stable, agent-readable way without breaking human CLI output.
  - **Verify:** CLI and MCP tests cover at least `aider`, `claude`, and `cursor` output descriptors.
  - **Depends on:** T3

### 3) Regression and dogfood cleanup

- [x] **T5: Add descriptor/emission ordering regression** *(~1h)* — Linear: COD-330
  - **Files:** `internal/usecase/*_test.go`, `internal/integration/*_test.go`
  - **Do:** assert descriptor order matches emitted top-level file ordering, with deterministic per-directory skill file expansion.
  - **Verify:** repeated sync returns identical path ordering.
  - **Depends on:** T1, T3

- [ ] **T6: Update dogfood docs/context to remove stale limitation** *(~45m)* — Linear: COD-331
  - **Files:** `AGENTS.md`, `CLAUDE.md`, `.creed/config/*`, `README.md` as appropriate
  - **Do:** remove the old "target-specific file semantics are crude" limitation and document the descriptor/rendering model accurately.
  - **Verify:** generated/dogfood context no longer contradicts implemented behavior.
  - **Depends on:** T1, T2

### 4) Delivery

- [ ] **T7: Full verification and OpenSpec finalization** *(~45m)* — Linear: COD-332
  - **Do:** run:
    - `go generate ./...`
    - `git diff --exit-code` after generation, accounting for intended docs/spec edits
    - `go build ./...`
    - `go test -race -count=1 ./...`
    - `go vet ./...`
    - `gofmt -l .`
  - **Verify:** all checks pass and task checkboxes reflect completed work.
  - **Depends on:** T1-T6
