# Proposal — unified-surface-generation

## Why

Creed's architecture intends `internal/service.Service` to be the single API contract for every interaction surface: CLI, MCP, and HTTP. The current implementation has the spine of that model, but not the payoff yet.

Today:

- the `Service` interface exists and is the right canonical boundary
- `go generate` emits CLI scaffolding and MCP metadata from that interface
- generated CLI commands still rely on handwritten/runtime per-method logic
- MCP still relies on a manual method switch, request structs, and schema definitions
- HTTP is referenced as a future surface but has no implementation

Adding a new capability is therefore still too manual. A developer can add a service method and get partial generated files, but must hand-wire CLI behavior, MCP request/schema handling, and any future HTTP endpoint separately. That is exactly the surface drift Creed was meant to avoid.

## What changes

- Introduce a first-class operation descriptor model generated from the `Service` interface and explicit metadata.
- Make CLI and MCP consume generated operation descriptors instead of parallel handwritten method maps.
- Add an HTTP JSON surface generated from the same operation descriptors.
- Add generator tests and fixture coverage proving a new operation can be added once and appear across CLI, MCP, and HTTP.
- Keep existing user-facing command names and current behavior compatible while migrating internals.

## Capabilities

- `service-interface`
- `http-surface`

## Impact

- Touches `internal/codegen/` substantially.
- Touches generated CLI files under `cmd/gen/`.
- Touches MCP server wiring under `internal/mcp/` and generated files under `internal/mcp/gen/`.
- Adds a new HTTP package/surface, likely under `internal/httpapi/` or `internal/http/`.
- Touches `main.go` `go:generate` outputs.
- Requires strong generation idempotency tests and full repo verification.

## Non-goals

- Do not redesign Creed's domain/usecase architecture.
- Do not add authentication, multi-tenant hosting, daemon lifecycle, or public deployment in this change.
- Do not remove existing CLI command names or break existing MCP tool names.
- Do not force REST purity; a generated JSON operation API is acceptable for the first HTTP surface.
