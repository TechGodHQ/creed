# Layered context source specification

## Requirements

### Requirement: Ordered source composition
The system SHALL read declared shared layers in manifest order and the local
consumer source last.

#### Scenario: Org and repo configs are emitted together
- Given a git layer with `org.md` and a local layer with `repo.md`
- When `creed sync` renders a context target
- Then both files are present in deterministic order
- And the org content precedes the repo content with a `---` separator

### Requirement: Shared source overrides are deterministic
The system SHALL use the later declaration when two layers use the same skill or
config name.

#### Scenario: Repository-specific skill override
- Given a shared and local skill with the same name
- When the composed source reads that skill
- Then the local skill content is used

### Requirement: Drift gating uses the composed source
`creed diff` SHALL compare target files with the fully composed source and SHALL
return a non-zero drift status when any owned output differs.

#### Scenario: Central context drift
- Given target output matching the org and repo layers
- When the central content changes
- Then `creed diff` reports the changed output and exits `1`

### Requirement: Layer health is validated
`creed validate` and `creed doctor` SHALL fetch/read every configured layer and
report unavailable or missing remote entries as structured diagnostics.

#### Scenario: Missing remote config
- Given a remote manifest referencing a missing config file
- When `creed validate` runs
- Then validation is invalid with a layer-source diagnostic
- And `creed doctor` includes the same error code

### Requirement: Pull is non-clobbering
`creed pull` SHALL persist the remote layer metadata and compose the pull with
local source files without replacing those files. Layered `push` SHALL be
rejected with an actionable message.
