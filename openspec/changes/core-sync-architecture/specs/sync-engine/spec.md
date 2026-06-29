## ADDED Requirements

### Requirement: SyncEngine SHALL orchestrate source-to-target synchronization

The system SHALL implement a `SyncEngine` in `internal/usecase/sync.go` that:
1. Reads the manifest via `SourceReader`
2. Reads all skills and configs referenced in the manifest
3. For each enabled target, resolves the emit paths and prepares files
4. Emits files via `TargetEmitter`
5. Returns a `SyncResult` summarizing the operation

#### Scenario: Sync emits all skills to all enabled targets
- **WHEN** `Sync` is called with a manifest that has 2 enabled targets and 3 skills
- **THEN** each target MUST receive all 3 skill files at their target-specific paths

#### Scenario: Sync skips disabled targets
- **WHEN** `Sync` is called and target "codex" is disabled in the manifest
- **THEN** no files MUST be emitted for the "codex" target

#### Scenario: Sync with specific target only emits to that target
- **WHEN** `Sync` is called with `SyncOptions{Target: "claude"}`
- **THEN** only the "claude" target MUST receive files

### Requirement: Sync SHALL be idempotent

Running `Sync` twice with the same source MUST produce identical results. The second run MUST not modify any files (all results should be "skipped").

#### Scenario: Second sync run skips all files
- **WHEN** `Sync` is called twice with the same source and targets
- **THEN** the second run MUST report all files as "skipped" with `FilesWritten: 0`

### Requirement: Sync SHALL return structured results

`SyncResult` SHALL contain per-target breakdowns with file counts and durations.

#### Scenario: SyncResult reports files written per target
- **WHEN** `Sync` writes 3 files to claude and 2 files to cursor
- **THEN** the `SyncResult` MUST contain entries for both targets with correct `FilesWritten` counts

#### Scenario: SyncResult reports error without aborting other targets
- **WHEN** one target's emit fails but others succeed
- **THEN** the failed target's `SyncResult.Error` MUST be populated AND successful targets MUST still be reported with their results

### Requirement: SyncOptions SHALL control sync behavior

The system SHALL define a `SyncOptions` struct that controls sync behavior with the following fields:
type SyncOptions struct {
    Target    string // empty = all enabled targets; non-empty = specific target only
    DryRun    bool   // if true, report what would change without writing
    Force     bool   // if true, overwrite even identical files
}
```

#### Scenario: DryRun reports changes without writing
- **WHEN** `Sync` is called with `SyncOptions{DryRun: true}`
- **THEN** the `SyncResult` MUST report which files would be written, but no files MUST be created or modified on disk

#### Scenario: Force overwrites identical files
- **WHEN** `Sync` is called with `SyncOptions{Force: true}` and a file already exists with identical content
- **THEN** the file MUST be overwritten and `EmitResult.Status` MUST be "written"
