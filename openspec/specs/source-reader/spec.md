# Spec — source-reader

## Purpose

Canonicalized requirements archived from the completed `core-sync-architecture` OpenSpec change.

## Requirements

### Requirement: SourceReader port SHALL define the source reading contract

The system SHALL define a `SourceReader` interface in `internal/ports/source.go` with the following methods:

```go
type SourceReader interface {
    ReadManifest(ctx context.Context) (*domain.Manifest, error)
    ReadSkill(ctx context.Context, name string) (*domain.Skill, error)
    ListSkills(ctx context.Context) ([]domain.SkillInfo, error)
    ReadConfig(ctx context.Context, name string) (*domain.ConfigFile, error)
    ListConfigs(ctx context.Context) ([]domain.ConfigInfo, error)
}
```

The interface MUST NOT expose filesystem paths, git internals, or HTTP details to callers.

#### Scenario: SourceReader returns manifest from local filesystem
- **WHEN** the LocalFS adapter's `ReadManifest` is called in a project with a valid `.creed/manifest.yaml`
- **THEN** it MUST return a populated `Manifest` struct with parsed YAML fields

#### Scenario: SourceReader returns error for missing manifest
- **WHEN** the LocalFS adapter's `ReadManifest` is called in a project without `.creed/manifest.yaml`
- **THEN** it MUST return an error with a message containing "manifest not found"

#### Scenario: SourceReader returns error for invalid manifest
- **WHEN** the LocalFS adapter's `ReadManifest` is called with a malformed `manifest.yaml`
- **THEN** it MUST return an error containing the parse error details

### Requirement: LocalFS source adapter SHALL read from .creed/ directory

The system SHALL implement a `LocalFSSource` adapter in `internal/adapters/localfs/source.go` that implements `SourceReader`.

The adapter SHALL:
- Read the manifest from `<project_root>/.creed/manifest.yaml`
- Read skills from paths relative to the `.creed/` directory
- Read configs from paths relative to the `.creed/` directory

#### Scenario: LocalFS reads skill content
- **WHEN** `ReadSkill(ctx, "code-review")` is called and `skills/code-review.md` exists in `.creed/`
- **THEN** it MUST return a `Skill` with the file's content as `[]byte`

#### Scenario: LocalFS returns error for missing skill
- **WHEN** `ReadSkill(ctx, "nonexistent")` is called
- **THEN** it MUST return an error containing "skill not found: nonexistent"

### Requirement: GitRemote source adapter SHALL clone and read from a git repository

The system SHALL implement a `GitRemoteSource` adapter in `internal/adapters/gitremote/source.go` that implements `SourceReader`.

The adapter SHALL:
- Clone the remote repository to a temporary directory on first access
- Cache the last-pulled commit SHA to skip redundant clones
- Support HTTPS URLs with optional token authentication
- Read the cloned repository using the same directory structure as LocalFS

#### Scenario: GitRemote clones and reads manifest
- **WHEN** `ReadManifest` is called on a GitRemoteSource pointing to a valid repository
- **THEN** it MUST clone the repository to a temp directory and return the parsed manifest

#### Scenario: GitRemote uses cache for unchanged remote
- **WHEN** `ReadManifest` is called twice on the same GitRemoteSource and the remote HEAD has not changed
- **THEN** the second call MUST NOT perform a new clone

#### Scenario: GitRemote returns error for unreachable repository
- **WHEN** `ReadManifest` is called on a GitRemoteSource pointing to a non-existent URL
- **THEN** it MUST return an error containing "failed to clone repository"
