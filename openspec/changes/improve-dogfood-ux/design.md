# Design — improve-dogfood-ux

## Overview

Dogfooding showed that Creed's current engine treats all targets as variations of the same shape:

```
configs[] ──aggregate──> first file path
skills[]  ──fanout─────> directory paths
```

That is enough for `AGENTS.md`, `CLAUDE.md`, and `.cursor/rules/`, but it breaks down for targets with multiple semantic files such as Aider (`.aider.conf.yml` + `CONVENTIONS.md`) and for future tool-specific surfaces.

The new model keeps the current simple path as the default, but introduces explicit output roles so target definitions can say what each output is for.

```
.creed source
  ├─ config/project.md      role: context
  ├─ config/development.md  role: context
  └─ skills/review.md       role: skill
          │
          ▼
   target mapper
          │
          ├─ claude: CLAUDE.md + .claude/skills/*.md
          ├─ codex:  AGENTS.md
          ├─ cursor: .cursor/rules/*.md
          └─ aider:  CONVENTIONS.md + .aider.conf.yml
```

## Design Decisions

### 1. Add output descriptors to target definitions

Current target definitions expose `EmitPaths(projectName) []string`. Replace or wrap this with a richer descriptor:

```go
type OutputKind string

const (
    OutputKindContext OutputKind = "context"
    OutputKindSkillDir OutputKind = "skill_dir"
    OutputKindConfig OutputKind = "config"
)

type TargetOutput struct {
    Path string
    Kind OutputKind
    Format string // markdown, yaml, text; initially advisory
}
```

Targets can still derive paths dynamically, but they return semantic outputs rather than bare strings.

**Alternative rejected:** encode everything in manifest entries. That would push target knowledge onto users and defeat Creed's value.

### 2. Preserve backward compatibility for existing manifests

Existing manifests with `skills` and `config` entries continue to work. Source entries do not need explicit roles yet. Creed infers:

- entries under `skills` → skill content
- entries under `config` → context content

Later changes can add richer per-entry roles if needed.

### 3. Keep mapping in the domain layer, rendering in the use case

Domain owns target metadata and output descriptors. The sync use case owns rendering source content into `ports.EmittedFile` candidates. Adapters only write files.

```
internal/domain    TargetOutput metadata
internal/usecase   prepareFiles/rendering
internal/adapters  filesystem writes only
```

This preserves the ports-and-adapters boundary.

### 4. Dry-run counts use the same aggregation as real emits

Dry-run should report `would_write`, `skipped`, and `failed` counts. Do not overload `FilesWritten` during dry-run; add an explicit count to the result model.

```go
type TargetResult struct {
    FilesWritten int
    FilesWouldWrite int
    FilesSkipped int
    FilesFailed int
}
```

CLI rendering chooses labels based on mode.

### 5. `creed init` scaffolds a useful project

`creed init <project>` should create:

```
.creed/manifest.yaml
.creed/config/project.md
.creed/config/development.md
.creed/skills/review.md
```

Defaults should enable `claude`, `codex`, and `cursor`, because those cover the common dogfood paths and produce visible useful output.

**Alternative rejected:** keep init minimal and document the rest. That made dogfooding too manual and is the opposite of a good first-run experience.

### 6. Dogfood regression uses a fixture, not the live repo

Tests should use a reduced Creed-like fixture under `testdata/` rather than depending on the repository's actual `.creed/` contents. The live dogfood files can evolve; the fixture locks the behavior contract.

## Interfaces / Contracts

### Domain

- Add `TargetOutput` and `OutputKind` types.
- Add `Outputs(projectName string) []TargetOutput` or equivalent to `Target`.
- Keep compatibility helpers for old path behavior until all callers migrate.

### Use case

- Replace bare `prepareFiles(target, skills, configs)` with descriptor-aware rendering.
- Render context outputs from ordered config entries.
- Render skill directory outputs from skill entries.
- Render config outputs for targets like Aider without stealing context content.

### CLI

- `creed sync --dry-run` summary must include `would_write` counts.
- `creed init` must be safe to rerun: existing files are not overwritten unless an explicit force option is added in a future change.

## External Dependencies

None. This change should use the existing standard library + current repo dependencies.

## Risks

- **Mapping complexity creep:** Keep roles small and target-owned. Do not add a template language yet.
- **Backward compatibility:** Existing manifests must continue emitting AGENTS/CLAUDE/Cursor outputs.
- **Aider semantics:** `.aider.conf.yml` may need minimal generated YAML. Keep it simple and tested.

## Verification Strategy

- Unit tests for target output descriptors and deterministic ordering.
- Unit tests for descriptor-aware file preparation.
- CLI tests for dry-run summary text.
- Init tests using temp dirs: files created, rerun does not overwrite edited files.
- Integration fixture: local `.creed` source emits expected AGENTS, CLAUDE, Cursor, and Aider files.
