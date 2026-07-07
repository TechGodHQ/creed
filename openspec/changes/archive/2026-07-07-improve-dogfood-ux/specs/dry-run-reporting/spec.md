# Spec — dry-run-reporting

## ADDED Requirements

### Requirement: Dry-run candidate counts

Creed SHALL report how many files would be written during dry-run separately from files actually written.

#### Scenario: dry-run has new files

- **Given** a target with three candidate files that do not exist on disk
- **When** `creed sync --dry-run` is executed
- **Then** the target result SHALL report `FilesWouldWrite == 3`
- **And** it SHALL report `FilesWritten == 0`
- **And** no files SHALL be written to disk

#### Scenario: dry-run has unchanged files

- **Given** a target with candidate files that already match disk contents
- **When** `creed sync --dry-run` is executed
- **Then** the target result SHALL report those files as skipped
- **And** it SHALL not count them as would-write

### Requirement: CLI dry-run summary accuracy

The `creed sync --dry-run` CLI SHALL display summary counts that match the per-file statuses printed below the summary.

#### Scenario: CLI dry-run lists would-write files

- **Given** dry-run output includes per-file `would_write` rows
- **When** the target summary is printed
- **Then** the summary SHALL include the number of would-write files
- **And** it SHALL NOT print `0 written` as the only change count for that target

#### Scenario: normal sync summary remains stable

- **Given** a normal non-dry-run sync
- **When** the target summary is printed
- **Then** the summary SHALL continue to report written, skipped, and failed counts
