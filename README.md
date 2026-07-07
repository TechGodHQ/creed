# creed

> One source of truth for AI context. Sync skills, specs, and config across every tool.

`creed` lets you define your AI assistant context — skills, specifications, and project
configuration — **once**, then emit it in the file layout each coding tool expects.

## Why?

Every AI coding tool has its own conventions:

| Tool | Context files |
|------|---------------|
| Claude Code | `CLAUDE.md`, `.claude/skills/` |
| Cursor | `.cursor/rules/` |
| Codex | `AGENTS.md` |
| Generic agents | `AGENTS.md` |
| Windsurf | `.windsurfrules` |
| Aider | `.aider.conf.yml`, `CONVENTIONS.md` |

Keeping those files in sync manually is fragile. `creed` makes the `.creed/`
directory the canonical source and emits target-specific files from it.

## Install

```bash
go install github.com/techgodhq/creed@latest
```

From a checkout:

```bash
go build ./...
go test ./...
```

## Quick start

```bash
# Initialize .creed/ in the current project
creed init my-project

# Edit the starter scaffold files
$EDITOR .creed/config/project.md
$EDITOR .creed/config/development.md
$EDITOR .creed/skills/review.md

# Emit context files for all enabled targets
creed sync

# Emit one target only
creed sync --target claude

# Preview candidate writes without touching the working tree
creed sync --target claude --dry-run

# Clean and rewrite emitted files for a target
creed sync --target claude --force
```

`creed init` is non-destructive: rerunning it creates missing starter files but
does not overwrite existing `.creed/` content.

By default, `creed init` creates:

- `.creed/manifest.yaml`
- `.creed/config/project.md`
- `.creed/config/development.md`
- `.creed/skills/review.md`

The generated manifest enables `claude`, `codex`, and `cursor` with
`output_dir: .`. Less universal targets (`agents`, `aider`, and `windsurf`) are
listed but disabled until you opt in.

## Manifest format

`creed` reads `.creed/manifest.yaml`:

```yaml
version: 1
source:
  type: local
  path: .creed
  # remote: https://github.com/example/context.git
targets:
  - name: agents
    enabled: false
    output_dir: .
  - name: aider
    enabled: false
    output_dir: .
  - name: claude
    enabled: true
    output_dir: .
  - name: codex
    enabled: true
    output_dir: .
  - name: cursor
    enabled: true
    output_dir: .
  - name: windsurf
    enabled: false
    output_dir: .
skills:
  - name: review
    path: skills/review.md
config:
  - name: project
    path: config/project.md
  - name: development
    path: config/development.md
```

Paths in `skills` and `config` are relative to `.creed/`. `output_dir` is relative
to the project root and is guarded so it cannot escape the project with `..` or
an absolute path.

## Source models

Local source is the default: Creed reads `.creed/` from the current project.
Git-backed sharing is available through the service `Pull` path: the git remote
is cloned or reused from cache, then read with the same manifest, skill, and
config semantics as a local source. The manifest can record the remote URL:

```yaml
source:
  type: git
  path: .creed
  remote: https://github.com/example/context.git
```

## Current sync behavior

- Skill files are emitted to target directory paths, such as `.claude/skills/`
  and `.cursor/rules/`.
- Config files are concatenated and emitted to context outputs, such as
  `CLAUDE.md`, `AGENTS.md`, `.windsurfrules`, or `CONVENTIONS.md`.
- Aider receives two files when enabled: `.aider.conf.yml` points Aider at
  `CONVENTIONS.md`, and `CONVENTIONS.md` receives the aggregated project
  context.
- The second identical run is idempotent and reports skipped files.
- `--dry-run` reports which candidate files would be written and which are
  already identical, without writing. Dry-run summaries include a separate
  `would_write` count, for example:

  ```text
  claude: 0 written, 2 would_write, 0 skipped, 0 failed
    would_write CLAUDE.md
    would_write .claude/skills/review.md
  ```

- `--force` cleans the target paths first, then rewrites emitted files.

## Architecture

Creed uses a ports-and-adapters layout:

- `internal/domain`: zero-dependency target registry and manifest/domain types.
- `internal/ports`: source-reader and target-emitter interfaces.
- `internal/adapters/localfs`: reads `.creed/` and writes target files locally.
- `internal/adapters/gitremote`: reads `.creed/` from a git remote clone/cache.
- `internal/usecase`: the sync engine and result model.
- `internal/service`: the canonical API shared by CLI, MCP, and future surfaces.
- `cmd`: Cobra CLI commands that delegate to `internal/service`.

See [`docs/architecture.md`](docs/architecture.md) for more detail.

## Verification

```bash
go test -race -count=1 ./...
go vet ./...
gofmt -l .
```

## License

MIT — see [LICENSE](LICENSE).
