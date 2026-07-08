## ADDED Requirements

### Requirement: Service interface SHALL be the single API surface

The system SHALL define a `Service` interface in `internal/service/service.go` that exposes all creed operations:

```go
type Service interface {
    Init(ctx context.Context, projectName string) error
    Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error)
    AddSkill(ctx context.Context, name, sourcePath string) error
    RemoveSkill(ctx context.Context, name string) error
    ListSkills(ctx context.Context) ([]SkillInfo, error)
    ListTargets(ctx context.Context) ([]TargetInfo, error)
    EnableTarget(ctx context.Context, name string) error
    DisableTarget(ctx context.Context, name string) error
    Pull(ctx context.Context, remoteURL string) error
    Push(ctx context.Context, remoteURL string) error
}
```

Every interaction surface (CLI, MCP, HTTP) SHALL be generated from this interface. No surface SHALL implement operations not present on the interface.

#### Scenario: Service interface covers all CLI commands
- **WHEN** the CLI generator runs
- **THEN** every method on `Service` MUST produce a corresponding Cobra command

#### Scenario: Service interface covers all MCP tools
- **WHEN** the MCP generator runs
- **THEN** every method on `Service` MUST produce a corresponding MCP tool registration

### Requirement: Code generator SHALL produce surface wrappers from the Service interface

The system SHALL implement a code generator in `internal/codegen/` that reads the `Service` interface via Go reflection and produces:
- Cobra CLI commands in `cmd/gen/` (one file per method)
- MCP tool definitions in `internal/mcp/gen/` (one file per method)

The generator SHALL be invoked via `go:generate` in `main.go`.

#### Scenario: Generator produces CLI command for Sync
- **WHEN** the generator processes the `Sync` method
- **THEN** it MUST produce a `cmd/gen/sync.go` file containing a Cobra command with flags derived from `SyncOptions`

#### Scenario: Generator produces MCP tool for AddSkill
- **WHEN** the generator processes the `AddSkill` method
- **THEN** it MUST produce a tool definition with JSON Schema parameters for `name` (string, required) and `sourcePath` (string, required)

#### Scenario: Generated code compiles without manual edits
- **WHEN** the generated files are compiled
- **THEN** they MUST pass `go vet ./...` without errors

### Requirement: CLI surface SHALL delegate to the Service interface

The generated CLI commands SHALL construct a `Service` implementation (wired with concrete adapters), call the corresponding method, and format the output for terminal display.

#### Scenario: CLI sync command calls Service.Sync
- **WHEN** `creed sync` is executed
- **THEN** it MUST call `Service.Sync` with options derived from CLI flags

#### Scenario: CLI init command calls Service.Init
- **WHEN** `creed init` is executed with a project name
- **THEN** it MUST call `Service.Init` and create `.creed/manifest.yaml`

### Requirement: MCP surface SHALL register tools from the Service interface

The generated MCP server SHALL register one tool per `Service` method, with:
- Tool name derived from the method name (e.g., `sync`, `add_skill`, `list_targets`)
- Input schema derived from Go struct tags on parameter types
- Description derived from the method's doc comment

#### Scenario: MCP server exposes sync tool
- **WHEN** an MCP client connects and lists tools
- **THEN** a tool named `sync` MUST be present with input schema matching `SyncOptions`

#### Scenario: MCP tool returns structured result
- **WHEN** an MCP client calls the `sync` tool
- **THEN** the response MUST contain the `SyncResult` serialized as structured JSON
