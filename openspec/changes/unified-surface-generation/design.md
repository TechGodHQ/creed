# Design — unified-surface-generation

## Current state

Creed has a useful but incomplete service-first architecture:

- `internal/service.Service` is the canonical API boundary.
- `main.go` has `go:generate` invoking `internal/codegen`.
- `internal/codegen` parses the service interface and emits CLI files plus MCP metadata.
- `cmd/gen/` contains generated Cobra command constructors and a generated runtime file.
- `internal/mcp/gen/` contains generated tool metadata.
- `internal/mcp/server.go` still manually maps method names to request schemas and handlers.
- HTTP is not implemented.

The result is partial generation. Adding a service operation still requires too much manual work per surface.

## Target model

```text
internal/service.Service
        ↓
codegen service parser + metadata resolver
        ↓
generated operation descriptors
        ↓
CLI renderer     MCP renderer     HTTP renderer
        ↓              ↓              ↓
cmd/gen       internal/mcp/gen   internal/httpapi/gen
```

The operation descriptor is the key abstraction. Surfaces should not independently rediscover command names, parameter names, schemas, or handler routing.

## Operation descriptor shape

A descriptor should include at least:

- service method name, e.g. `Sync`
- canonical operation name, e.g. `sync`
- description from doc comment or metadata
- input fields
  - external name, e.g. `dry_run`
  - Go field/parameter source
  - primitive type or supported struct type
  - required/optional
  - CLI flag/arg mapping
  - help text/default if available
- output metadata
- per-surface names/routes
  - CLI command name
  - MCP tool name
  - HTTP route

Implementation may choose generated Go structs rather than a runtime descriptor DSL, but there must be a single generated source of truth consumed by all generated surfaces.

## Input model recommendation

The current `Service` interface mixes shapes:

- simple positional params: `AddSkill(ctx, name, sourcePath string)`
- struct params: `Sync(ctx, opts usecase.SyncOptions)`
- no-input methods: `ListTargets(ctx)`

For short-term compatibility, support the existing shapes. For long-term generation quality, prefer request DTOs for new operations:

```go
type AddSkillRequest struct {
    Name string `json:"name" cli:"arg,required" help:"Skill name"`
    SourcePath string `json:"source_path,omitempty" cli:"arg,optional" help:"Source skill file path"`
}
```

This change should not force all existing methods to migrate immediately, but it should establish the generator path for DTO-backed operations.

## CLI generation

CLI generation should preserve existing names and UX. It can support:

- positional args for fields marked as args
- flags for optional fields and booleans
- stable command names derived from descriptors
- generated delegation to `Service` methods

If a method shape cannot be generated safely, generation should fail or emit an explicit skip record. Silent partial commands are not acceptable.

## MCP generation

MCP should no longer need a handwritten method-name switch for every operation. Generated code should include:

- tool definitions
- JSON schema/input options
- request decoding
- service method invocation
- structured result envelope

A small handwritten MCP runtime is fine; per-operation behavior should be generated.

## HTTP generation

Prefer generated JSON operation routes over hand-designed REST for this phase. A simple and stable shape is enough:

- `GET /v1/operations` — operation catalog
- `POST /v1/operations/{operation}` — call an operation with JSON input

The generated handler should be usable as an `http.Handler` with a supplied `service.Service`.

## Testing strategy

- generator unit tests for descriptor extraction
- golden or fixture tests proving generated CLI/MCP/HTTP files exist for all service operations
- in-process CLI command tests with fake service
- in-process MCP tests with fake service
- `httptest` HTTP tests with fake service
- idempotency test: `go generate ./...` then clean diff

## Migration strategy

1. Generate shared descriptors while preserving current generated CLI/MCP behavior.
2. Move MCP per-operation handlers into generated code.
3. Move CLI runtime per-operation mapping into descriptor-driven/generated code.
4. Add generated HTTP surface.
5. Add fixture proving new-operation golden path.
6. Update docs/specs and archive.

## Non-goals

- Authn/authz.
- Long-running daemon process.
- Public hosted API.
- Full OpenAPI generation, unless it falls out cheaply from descriptors.
