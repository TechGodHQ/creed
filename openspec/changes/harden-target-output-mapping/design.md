# Design — harden-target-output-mapping

## Current state

Creed already has:

- `domain.TargetOutput` with `Path`, `Kind`, and `Format`
- output descriptors for all supported targets
- descriptor-aware `prepareFiles` logic in `internal/usecase/sync.go`
- minimal Aider config rendering
- dogfood integration coverage for a reduced fixture

The remaining problem is product hardening: the descriptors are not fully visible to users/agents, target-specific renderers are still coupled into sync orchestration, and stale docs/context still describe the limitation as if it were unresolved.

## Approach

### 1. Keep descriptors as the domain contract

Do not replace `TargetOutput`. Extend only where needed. The minimal descriptor shape remains:

- path
- kind
- format

If implementation needs richer renderer identity, prefer an additive field such as `Renderer` or a private registry keyed by target/kind over breaking existing callers.

### 2. Move rendering decisions out of orchestration

`SyncEngine` should orchestrate source reads and target emission. Rendering should be delegated to focused helpers or renderer registry functions:

- context renderer
- skill directory renderer
- config renderer
- Aider config renderer

This makes unknown output kinds explicit failures instead of silent behavior.

### 3. Expose descriptors through target listing

`service.ListTargets` and generated CLI/MCP surfaces should expose output descriptors, not only legacy `EmitPaths`. Keep `EmitPaths` for compatibility, but add structured output details for agents.

CLI output can stay human-readable; JSON/structured internals should carry path/kind/format.

### 4. Regression focus

Tests should prove:

- all targets expose descriptors
- descriptors match emitted file ordering
- Aider emits both `.aider.conf.yml` and `CONVENTIONS.md` correctly
- repeated sync is stable
- docs/context no longer claim target mapping is crude

## Non-goals

- Do not add new target families in this change.
- Do not redesign the manifest schema.
- Do not require users to annotate output roles in existing manifests.
- Do not remove `EmitPaths` until compatibility callers are gone.

## Verification

Run:

```bash
go generate ./...
git diff --exit-code
go build ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l .
```
