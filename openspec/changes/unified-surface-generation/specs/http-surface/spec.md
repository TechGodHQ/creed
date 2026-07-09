# Spec — http-surface

## ADDED Requirements

### Requirement: HTTP surface SHALL expose Service operations through generated handlers

Creed SHALL provide an HTTP JSON surface generated from the same operation descriptors as CLI and MCP.

#### Scenario: HTTP surface lists generated operations
- **WHEN** an HTTP client requests the operation catalog endpoint
- **THEN** the response SHALL list generated operations with names, descriptions, input schemas, and routes

#### Scenario: HTTP sync operation delegates to Service.Sync
- **WHEN** an HTTP client calls the generated sync operation with JSON input
- **THEN** the handler SHALL decode the request, call `Service.Sync`, and return the structured `SyncResult`

#### Scenario: HTTP service errors are structured
- **WHEN** a Service method returns an error
- **THEN** the HTTP surface SHALL return a non-2xx status with a structured JSON error envelope
- **AND** the envelope SHALL include the operation name and error message

### Requirement: HTTP API shape SHALL be generator-friendly before REST purity

The first HTTP surface SHALL prefer a consistent generated operation API over hand-designed REST endpoints.

#### Scenario: operation route is deterministic
- **WHEN** the generator processes a service method named `ListTargets`
- **THEN** the HTTP route SHALL be deterministic, such as `POST /v1/operations/list_targets` or an explicitly documented equivalent

#### Scenario: no custom endpoint wiring for new operation
- **GIVEN** a supported new Service operation
- **WHEN** code generation runs
- **THEN** the HTTP route and handler SHALL be generated without adding handwritten route registration code

### Requirement: HTTP surface SHALL be testable without starting a daemon

Creed SHALL expose the HTTP handler as a normal `http.Handler` that can be tested in-process.

#### Scenario: httptest can call generated handler
- **WHEN** a test creates the generated HTTP handler with a fake Service
- **THEN** it SHALL be able to call operations using `httptest` without starting a long-lived process
