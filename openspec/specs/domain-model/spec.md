# Spec — domain-model

## Purpose

Canonicalized requirements archived from the completed `core-sync-architecture` OpenSpec change.

## Requirements

### Requirement: Domain types SHALL represent the core vocabulary

The system SHALL define the following domain types in `internal/domain/`:

- `Skill` — represents an AI skill file. Fields: `Name string`, `Path string`, `Content []byte`
- `Spec` — represents a specification file. Fields: `Name string`, `Path string`, `Content []byte`
- `ConfigFile` — represents a configuration/context file. Fields: `Name string`, `Path string`, `Content []byte`
- `Target` — represents a target tool. Fields: `Name string`, `DisplayName string`, `EmitPaths func(projectName string) []string`
- `Manifest` — the project configuration. Fields: `Version int`, `Source SourceConfig`, `Targets []TargetConfig`, `Skills []SkillEntry`, `Configs []ConfigEntry`
- `SyncResult` — result of a sync operation. Fields: `Target string`, `FilesWritten int`, `FilesSkipped int`, `Duration time.Duration`, `Error error`
- `SourceConfig` — source backend configuration. Fields: `Type string` (local|git), `Path string`, `Remote string`

#### Scenario: Domain package has zero external imports
- **WHEN** the `internal/domain` package is compiled
- **THEN** it MUST NOT import any package outside the Go standard library

#### Scenario: Target EmitPaths returns expected paths for claude
- **WHEN** `Target{Name: "claude"}.EmitPaths("myproject")` is called
- **THEN** it MUST return `["CLAUDE.md", ".claude/skills/"]`

#### Scenario: Manifest version field defaults to 1
- **WHEN** a new Manifest is created via `Init`
- **THEN** the `Version` field MUST be set to `1`
