# Spec — target-output-mapping

## ADDED Requirements

### Requirement: Target output descriptors

Creed SHALL represent target outputs as semantic descriptors rather than bare file paths.

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

### Requirement: Deterministic output ordering

Creed SHALL produce deterministic emitted file ordering for every target.

#### Scenario: repeated sync candidate generation

- **Given** the same manifest and source contents
- **When** emitted file candidates are generated twice
- **Then** the returned file list SHALL have the same paths in the same order
