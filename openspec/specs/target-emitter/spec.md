# Spec — target-emitter

## Purpose

Canonicalized requirements archived from the completed `core-sync-architecture` OpenSpec change.

## Requirements

### Requirement: TargetEmitter port SHALL define the emit contract

The system SHALL define a `TargetEmitter` interface in `internal/ports/emitter.go` with the following methods:

```go
type EmittedFile struct {
    Path    string
    Content []byte
}

type EmitResult struct {
    Path   string
    Status string // "written" | "skipped" | "error"
    Error  error
}

type TargetEmitter interface {
    Emit(ctx context.Context, target domain.Target, files []EmittedFile) ([]EmitResult, error)
    Clean(ctx context.Context, target domain.Target) error
}
```

#### Scenario: Emit writes files to target-specific paths
- **WHEN** `Emit` is called for target "claude" with files `["CLAUDE.md", ".claude/skills/code-review.md"]`
- **THEN** the files MUST be written to the project root at those relative paths

#### Scenario: Clean removes emitted files for a target
- **WHEN** `Clean` is called for target "cursor"
- **THEN** all files under `.cursor/rules/` MUST be removed

### Requirement: LocalFS emitter adapter SHALL write files to the project filesystem

The system SHALL implement a `LocalFSEmitter` adapter in `internal/adapters/localfs/emitter.go` that implements `TargetEmitter`.

The adapter SHALL:
- Create parent directories as needed (`os.MkdirAll`)
- Write files with `0644` permissions
- Return `EmitResult` per file indicating written/skipped/error status

#### Scenario: Emitter creates nested directories
- **WHEN** `Emit` writes to `.claude/skills/code-review.md` and `.claude/skills/` does not exist
- **THEN** the directory `.claude/skills/` MUST be created before writing

#### Scenario: Emitter skips identical files
- **WHEN** `Emit` is called for a file that already exists with identical content
- **THEN** the `EmitResult.Status` for that file MUST be "skipped"

#### Scenario: Emitter overwrites changed files
- **WHEN** `Emit` is called for a file that exists with different content
- **THEN** the file MUST be overwritten and `EmitResult.Status` MUST be "written"

### Requirement: Target registry SHALL map target names to definitions

The system SHALL maintain a target registry with the following initial targets:

| Target | Emit Paths |
|--------|-----------|
| claude | `CLAUDE.md`, `.claude/skills/` |
| cursor | `.cursor/rules/` |
| codex | `AGENTS.md` |
| agents | `AGENTS.md` |
| windsurf | `.windsurfrules` |
| aider | `.aider.conf.yml`, `CONVENTIONS.md` |

#### Scenario: Registry returns all registered targets
- **WHEN** all targets are listed
- **THEN** the registry MUST contain exactly: claude, cursor, codex, agents, windsurf, aider

#### Scenario: Registry returns error for unknown target
- **WHEN** a target named "unknown" is looked up
- **THEN** the registry MUST return an error containing "unknown target: unknown"
