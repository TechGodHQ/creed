# Spec — service-interface

## MODIFIED Requirements

### Requirement: Service interface SHALL be the single API surface

The system SHALL define a `Service` interface in `internal/service/service.go` that exposes all Creed operations. Every interaction surface (CLI, MCP, HTTP) SHALL be generated from this interface or from operation descriptors generated from this interface. No surface SHALL implement operations not present on the interface unless explicitly marked internal-only.

#### Scenario: Service interface covers all CLI commands
- **WHEN** the CLI generator runs
- **THEN** every public method on `Service` MUST produce a corresponding Cobra command or an explicit generated skip record with reason

#### Scenario: Service interface covers all MCP tools
- **WHEN** the MCP generator runs
- **THEN** every public method on `Service` MUST produce a corresponding MCP tool registration or an explicit generated skip record with reason

#### Scenario: Service interface covers all HTTP operations
- **WHEN** the HTTP generator runs
- **THEN** every public method on `Service` MUST produce a corresponding HTTP operation or an explicit generated skip record with reason

### Requirement: Code generator SHALL produce surface wrappers from the Service interface

The system SHALL implement a code generator in `internal/codegen/` that reads the `Service` interface and produces:

- shared operation descriptors
- Cobra CLI command wrappers in `cmd/gen/`
- MCP tool definitions/handlers in `internal/mcp/gen/`
- HTTP operation route definitions/handlers in the generated HTTP surface package

The generator SHALL be invoked via `go:generate` in `main.go`.

#### Scenario: Generator produces operation descriptor for Sync
- **WHEN** the generator processes the `Sync` method
- **THEN** it MUST produce an operation descriptor containing name, description, input fields, output type metadata, CLI name, MCP name, and HTTP route metadata

#### Scenario: Generator produces CLI command for Sync
- **WHEN** the generator processes the `Sync` method
- **THEN** it MUST produce a Cobra command with flags derived from the operation input descriptor
- **AND** the command MUST delegate to `Service.Sync`

#### Scenario: Generator produces MCP tool for AddSkill
- **WHEN** the generator processes the `AddSkill` method
- **THEN** it MUST produce a callable MCP tool with JSON Schema parameters for `name` and `source_path`
- **AND** no handwritten switch entry SHALL be required for the tool to call `Service.AddSkill`

#### Scenario: Generated code compiles without manual edits
- **WHEN** `go generate ./...` is run
- **THEN** a subsequent `git diff --exit-code` SHALL show no changes if generated files are already current
- **AND** `go vet ./...` SHALL pass

### Requirement: CLI surface SHALL delegate to the Service interface

Generated CLI commands SHALL construct or receive a `Service` implementation, parse operation inputs from args/flags, call the corresponding method, and format terminal output.

#### Scenario: CLI sync command calls Service.Sync
- **WHEN** `creed sync --target claude --dry-run` is executed
- **THEN** it MUST call `Service.Sync` with options derived from CLI flags
- **AND** it MUST preserve the existing dry-run output semantics

#### Scenario: CLI existing names remain stable
- **WHEN** generated CLI commands are registered
- **THEN** existing command names such as `sync`, `init`, `list-targets`, and `add-skill` MUST remain stable

### Requirement: MCP surface SHALL register tools from generated operation descriptors

The generated MCP server SHALL register one tool per service operation descriptor, with:

- tool name derived from operation metadata
- input schema derived from operation input fields
- description derived from the service method's doc comment or explicit metadata
- handler generated to decode payloads and call the matching `Service` method

#### Scenario: MCP server exposes sync tool
- **WHEN** an MCP client connects and lists tools
- **THEN** a tool named `sync` MUST be present with input schema matching `SyncOptions`

#### Scenario: MCP tool returns structured result
- **WHEN** an MCP client calls the `sync` tool
- **THEN** the response MUST contain the `SyncResult` serialized as structured JSON

#### Scenario: New service method is callable over MCP after generation
- **GIVEN** a fixture service interface with a new method using supported input and output shapes
- **WHEN** code generation runs
- **THEN** the generated MCP surface MUST expose and call the new method without adding handwritten server switch cases

## ADDED Requirements

### Requirement: Operation descriptors SHALL be the shared generated contract

Creed SHALL generate a shared operation descriptor set from the `Service` interface and supported metadata so all surfaces consume the same operation names, input fields, output metadata, and documentation.

#### Scenario: descriptors include stable surface names
- **WHEN** descriptors are generated
- **THEN** each descriptor SHALL include stable names for service method, CLI command, MCP tool, and HTTP route

#### Scenario: descriptors include supported input fields
- **WHEN** a service method has supported input parameters or request structs
- **THEN** the descriptor SHALL include field names, types, required/optional status, defaults where known, and help text where available

#### Scenario: unsupported shapes fail explicitly
- **WHEN** a service method uses an input/output shape the generator cannot map
- **THEN** generation SHALL fail with a clear error or emit an explicit skip record
- **AND** it SHALL NOT produce silently unusable surface code

### Requirement: New operation golden path SHALL be tested

Creed SHALL include generator fixture coverage proving that a new operation can be defined once and become available across generated CLI, MCP, and HTTP surfaces.

#### Scenario: fixture operation appears across all generated surfaces
- **GIVEN** a fixture `Service` interface containing a representative new operation
- **WHEN** the generator runs against the fixture
- **THEN** generated CLI, MCP, and HTTP code SHALL compile
- **AND** tests SHALL prove the generated surfaces delegate to the corresponding service method
