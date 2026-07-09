# Spec — target-output-mapping

## MODIFIED Requirements

### Requirement: Target output descriptors

Creed SHALL represent target outputs as semantic descriptors rather than bare file paths, and descriptors SHALL be inspectable through supported list-target surfaces.

#### Scenario: target declares context and skill outputs

- **Given** the `claude` target definition
- **When** its outputs are requested
- **Then** it SHALL include a context output for `CLAUDE.md`
- **And** it SHALL include a skill directory output for `.claude/skills/`

#### Scenario: target declares multiple semantic file outputs

- **Given** the `aider` target definition
- **When** its outputs are requested
- **Then** it SHALL distinguish `.aider.conf.yml` from `CONVENTIONS.md` by output kind
- **And** it SHALL NOT rely on bare path order alone to decide content

#### Scenario: target descriptors are inspectable

- **Given** a user or agent asks Creed to list supported targets
- **When** target details are returned through service, CLI, or MCP list-target surfaces
- **Then** each target SHALL include its output descriptors with path, kind, and format
- **And** the descriptor order SHALL match sync emission order

### Requirement: Backward-compatible manifest rendering

Creed SHALL continue to render existing manifests that use `skills` and `config` entries without requiring new manifest fields.

#### Scenario: legacy config entries render to context outputs

- **Given** a manifest with config entries and no explicit output roles
- **When** sync renders a target with a context output
- **Then** Creed SHALL aggregate the config entries into the context output deterministically

#### Scenario: legacy skill entries render to skill directory outputs

- **Given** a manifest with skill entries and no explicit output roles
- **When** sync renders a target with a skill directory output
- **Then** Creed SHALL emit one markdown file per skill into that directory

#### Scenario: unknown output kinds fail safely

- **Given** a target definition contains an output kind without a registered renderer
- **When** sync prepares emitted files for that target
- **Then** Creed SHALL return a structured target-level error
- **And** it SHALL NOT silently skip or misroute content

### Requirement: Deterministic output ordering

Creed SHALL produce deterministic emitted file ordering for every target.

#### Scenario: repeated sync candidate generation

- **Given** the same manifest and source contents
- **When** emitted file candidates are generated twice
- **Then** the returned file list SHALL have the same paths in the same order

#### Scenario: descriptor inspection and sync agree

- **Given** a target's output descriptors are listed
- **When** sync emits files for that target from a fixed manifest
- **Then** emitted top-level file paths SHALL follow descriptor order before per-directory skill file expansion

## ADDED Requirements

### Requirement: Target-specific renderer ownership

Creed SHALL route output rendering through explicit renderer functions keyed by output kind and target needs.

#### Scenario: Aider config renderer owns `.aider.conf.yml`

- **Given** the `aider` target has config and context outputs
- **When** sync prepares files for Aider
- **Then** the Aider config renderer SHALL emit deterministic YAML that references the context output
- **And** the context output SHALL contain aggregated config content

#### Scenario: context renderer owns aggregated context files

- **Given** a target has a context output
- **When** sync prepares files from multiple config entries
- **Then** the context renderer SHALL aggregate config content in manifest order with deterministic separators

### Requirement: Dogfood clean-tree regression

Creed SHALL include a regression test or CI guard proving dogfood target output mappings stay stable.

#### Scenario: dogfood sync leaves generated context stable

- **Given** Creed's dogfood fixture or repository context source
- **When** Creed sync is run for supported targets
- **Then** expected target files SHALL be present
- **And** repeating sync SHALL not change generated content unexpectedly
