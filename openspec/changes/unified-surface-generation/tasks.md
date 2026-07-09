# Tasks — unified-surface-generation

## Goal

Make Creed's Service interface the real single source of truth for generated CLI, MCP, and HTTP surfaces, so adding a supported operation requires one service definition plus generation instead of per-surface hand wiring.

## Task order rule

Descriptor model first, then migrate MCP/CLI onto it, then add HTTP, then prove the new-operation golden path, then docs/final verification.

## Tasks

### 1) Operation descriptor foundation

- [ ] **T1: Generate shared operation descriptors from Service** *(~2h)* — Linear: COD-333
  - **Files:** `internal/codegen/`, generated descriptor package/files, tests
  - **Do:** extract service method name, operation name, doc comment, params/input shape, output metadata, and stable per-surface names into a generated descriptor set.
  - **Verify:** generator tests assert descriptors for `Sync`, `AddSkill`, `ListTargets`, and no-input/simple-param/struct-param shapes.

- [ ] **T2: Define supported input-shape rules and explicit failure behavior** *(~1h 30m)* — Linear: COD-334
  - **Files:** `internal/codegen/`, tests, `openspec/changes/unified-surface-generation/design.md` if needed
  - **Do:** document and enforce supported shapes: `context.Context`, simple params, struct option/request params with JSON tags, no-input methods. Unsupported shapes must fail or emit explicit skip records.
  - **Verify:** fixture tests cover unsupported shapes and produce clear errors/skips.
  - **Depends on:** T1

### 2) MCP generation hardening

- [ ] **T3: Generate MCP schemas and handlers from operation descriptors** *(~2h 30m)* — Linear: COD-335
  - **Files:** `internal/codegen/`, `internal/mcp/gen/`, `internal/mcp/server.go`, MCP tests
  - **Do:** move per-operation request decoding/schema/handler wiring out of handwritten `server.go` switches into generated MCP code or descriptor-driven runtime.
  - **Verify:** adding a fixture operation exposes a callable MCP tool without adding a handwritten switch case.
  - **Depends on:** T1, T2

### 3) CLI generation hardening

- [ ] **T4: Generate CLI args/flags/delegation from operation descriptors** *(~2h 30m)* — Linear: COD-336
  - **Files:** `internal/codegen/`, `cmd/gen/`, `cmd/generated.go`, CLI tests
  - **Do:** make CLI commands consume descriptors for names, args, flags, help text, and service delegation while preserving existing command names and output semantics.
  - **Verify:** `sync`, `init`, `add-skill`, `list-targets` behavior remains compatible; fixture operation generates a callable CLI command.
  - **Depends on:** T1, T2

### 4) HTTP surface

- [ ] **T5: Add generated HTTP operation surface** *(~2h 30m)* — Linear: COD-337
  - **Files:** new `internal/httpapi/` or equivalent, `internal/codegen/`, generated HTTP files, tests
  - **Do:** add `http.Handler` construction backed by `service.Service`; generate operation catalog and JSON operation call routes from descriptors.
  - **Verify:** `httptest` can list operations and call `sync`/`list_targets` using a fake Service.
  - **Depends on:** T1, T2

- [ ] **T6: Standardize error envelopes across MCP and HTTP** *(~1h)* — Linear: COD-338
  - **Files:** MCP runtime, HTTP runtime, shared generated/runtime helpers as appropriate, tests
  - **Do:** ensure service errors produce structured envelopes with operation name, ok/error state, and message across MCP and HTTP.
  - **Verify:** tests cover service error behavior for both surfaces.
  - **Depends on:** T3, T5

### 5) Golden path and generation idempotency

- [ ] **T7: Add new-operation golden path fixture** *(~2h)* — Linear: COD-339
  - **Files:** `internal/codegen` test fixtures, generated temp outputs, CLI/MCP/HTTP tests
  - **Do:** create a fixture `Service` with a representative new operation and prove generated CLI, MCP, and HTTP surfaces compile and delegate without handwritten per-surface mappings.
  - **Verify:** fixture fails if any surface requires manual switch/registration work.
  - **Depends on:** T3, T4, T5

- [ ] **T8: Add generation idempotency guard** *(~1h)* — Linear: COD-340
  - **Files:** codegen tests or CI script/workflow as appropriate
  - **Do:** ensure `go generate ./...` is idempotent and generated files are committed/current.
  - **Verify:** `go generate ./... && git diff --exit-code` passes from a clean tree.
  - **Depends on:** T3, T4, T5

### 6) Docs and finalization

- [ ] **T9: Update architecture docs for generated surfaces** *(~1h)* — Linear: COD-341
  - **Files:** `README.md`, `AGENTS.md`, `.creed/config/*`, OpenSpec specs as appropriate
  - **Do:** explain Service → operation descriptor → CLI/MCP/HTTP generation flow and current extension workflow.
  - **Verify:** docs no longer describe HTTP as merely future once implemented.
  - **Depends on:** T5, T7

- [ ] **T10: Full verification and OpenSpec finalization** *(~45m)* — Linear: COD-342
  - **Do:** run:
    - `go generate ./...`
    - `git diff --exit-code`
    - `go build ./...`
    - `go test -race -count=1 ./...`
    - `go vet ./...`
    - `gofmt -l .`
  - **Verify:** all checks pass and all task checkboxes reflect completed work.
  - **Depends on:** T1-T9
