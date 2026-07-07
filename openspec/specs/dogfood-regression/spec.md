# Spec — dogfood-regression

## Purpose

Define regression coverage for Creed dogfooding workflows so first-run target rendering stays grounded in a realistic `.creed/` source fixture.

## Requirements

### Requirement: Dogfood fixture coverage

Creed SHALL include a reduced dogfood fixture that represents a real `.creed/` source context.

#### Scenario: fixture contains source context

- **Given** the dogfood regression fixture
- **When** the fixture is inspected
- **Then** it SHALL include a manifest
- **And** it SHALL include at least two config entries
- **And** it SHALL include at least one skill entry

### Requirement: Dogfood fixture emits expected files

Creed SHALL verify that the dogfood fixture emits expected files for supported first-run targets.

#### Scenario: sync dogfood fixture

- **Given** the dogfood regression fixture
- **When** sync runs against a temporary output directory
- **Then** Creed SHALL emit `AGENTS.md`
- **And** it SHALL emit `CLAUDE.md`
- **And** it SHALL emit at least one `.claude/skills/*.md` file
- **And** it SHALL emit at least one `.cursor/rules/*.md` file

### Requirement: Dogfood fixture covers richer target mapping

The dogfood regression SHALL cover at least one target with multiple semantic file outputs.

#### Scenario: sync Aider target from fixture

- **Given** the dogfood regression fixture includes the `aider` target enabled
- **When** sync runs against a temporary output directory
- **Then** Creed SHALL emit `.aider.conf.yml`
- **And** it SHALL emit `CONVENTIONS.md`
- **And** those files SHALL contain content appropriate to their output kinds

### Requirement: Dogfood regression is idempotent

The dogfood regression SHALL verify repeated sync stability.

#### Scenario: sync fixture twice

- **Given** the dogfood regression fixture has already been synced once
- **When** sync runs a second time with the same source
- **Then** all unchanged files SHALL be reported as skipped
- **And** no emitted content SHALL change
