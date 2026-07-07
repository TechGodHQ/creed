# Spec — init-scaffolding

## Purpose

Define the first-run project scaffold created by `creed init`, including default source files, target defaults, and non-destructive rerun behavior.

## Requirements

### Requirement: Useful init scaffold

`creed init` SHALL create a useful `.creed/` source tree suitable for immediate dogfooding.

#### Scenario: initialize empty project

- **Given** an empty project directory
- **When** `creed init my-project` is executed
- **Then** Creed SHALL create `.creed/manifest.yaml`
- **And** it SHALL create `.creed/config/project.md`
- **And** it SHALL create `.creed/config/development.md`
- **And** it SHALL create `.creed/skills/review.md`

### Requirement: Sane default targets

`creed init` SHALL enable practical default targets for the initial scaffold.

#### Scenario: inspect generated manifest

- **Given** `creed init my-project` completed successfully
- **When** `.creed/manifest.yaml` is read
- **Then** the manifest SHALL enable `claude`, `codex`, and `cursor` by default
- **And** all default target output directories SHALL be the project root (`.`)

### Requirement: Init is non-destructive

`creed init` SHALL NOT overwrite existing user-authored files by default.

#### Scenario: rerun init after editing project config

- **Given** `.creed/config/project.md` already exists with custom content
- **When** `creed init my-project` is executed again
- **Then** Creed SHALL preserve the existing file content
- **And** it SHALL return a result or message indicating the file was skipped

### Requirement: Scaffolded project syncs without manual edits

The initial scaffold SHALL be syncable immediately.

#### Scenario: sync after init

- **Given** `creed init my-project` completed in an empty project
- **When** `creed sync --dry-run` is executed
- **Then** Creed SHALL produce at least one candidate output for each enabled default target
- **And** the command SHALL exit successfully
