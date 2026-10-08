# creed

> One source of truth for AI context. Sync skills, specs, and config across every tool.

`creed` lets you define your AI assistant context — skills, specifications, and project
configuration — **once**, then emit it in the file layout each coding tool expects.

## Why?

Every AI coding tool has its own conventions:

| Tool | Context files |
|------|---------------|
| Claude Code | `CLAUDE.md`, `.claude/skills/` |
| GitHub Copilot | `.github/copilot-instructions.md` |
| Cursor | `.cursor/rules/` |
| Codex | `AGENTS.md` |
| Gemini CLI | `GEMINI.md`, `.gemini/` |
| OpenCode | `AGENTS.md`, `.opencode/agents/` |
| Generic agents | `AGENTS.md` |
| Windsurf | `.windsurfrules` |
| Aider | `.aider.conf.yml`, `CONVENTIONS.md` |

Keeping those files in sync manually is fragile. `creed` makes the `.creed/`
directory the canonical source and emits target-specific files from it.

## Install

```bash
go install github.com/techgodhq/creed@latest
```

For a pinned release:

```bash
go install github.com/techgodhq/creed@v0.5.0
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

# Watch .creed/ and re-sync on every save (Ctrl-C to stop)
creed watch

# Watch only one target, with a custom debounce window
creed watch --target claude --debounce 250ms

# Quiet mode: report only errors
creed watch --quiet

# Check the manifest and referenced source files without writing outputs
creed validate

# Diagnose the project setup: root, manifest, targets, git availability
creed doctor

# Check rendered output for CI drift (exit 1 when output differs)
creed check
creed check --target claude

# Preview the same line-level changes without treating it as a CI gate
creed diff
creed diff --target claude

# Manage manifest registrations without hand-editing YAML
creed add-skill review skills/review.md
creed remove-skill review
creed list-skills
creed add-config project config/project.md
creed remove-config project
creed list-configs
creed enable-target gemini
creed disable-target aider
```

`creed init` is non-destructive: rerunning it creates missing starter files but
does not overwrite existing `.creed/` content.

By default, `creed init` creates:

- `.creed/manifest.yaml`
- `.creed/config/project.md`
- `.creed/config/development.md`
- `.creed/skills/review.md`

The generated manifest enables `claude`, `codex`, and `cursor` with
`output_dir: .`. Less universal targets (`agents`, `aider`, `gemini`, and
`windsurf`) are listed but disabled until you opt in.
Newer targets (`copilot`, `opencode`) are scaffolded disabled as well and can
be enabled with `creed enable-target`.

## Agent guide

Creed's unit of work is a project root containing `.creed/manifest.yaml`. The
manifest declares a source and enabled targets; `sync` resolves that source and
emits deterministic target outputs. An agent can discover the contract without
reading Go source:

1. Run `creed list-targets` anywhere to inspect every supported target and its
   emitted paths (`path|kind|format`). Without a manifest every target is shown
   as disabled; this is global capability discovery, not project inspection.
2. In a project, run `creed list-skills`, `creed validate`, and `creed diff`.
   `list-skills` outside a project succeeds with no registrations.
3. Use `creed sync --dry-run` before a write, then `creed sync`; use `creed
   check` as the nonzero CI drift gate after generation. `creed diff` prints
   the same stable unified diff for interactive inspection.
4. Run `creed doctor` for setup *and generated-output* health. It exits
   nonzero when rendered enabled-target output is missing, modified, or stale;
   inspect the exact change with `creed diff`, then repair it with `creed sync`.

For a noninteractive CI gate:

```bash
creed validate && creed check
```

### Source and command behavior

| Source type | `sync`, `diff`, `validate`, `doctor` | `pull` | `push` |
|---|---|---|---|
| `local` | Reads `.creed/` in the project. | Requires a remote argument and converts the project to layered source. | Publishes the configured local source. |
| `git` | Reads the configured remote through the git source/cache. | Refreshes/syncs the configured remote. | Publishes the configured remote source. |
| `layered` | Resolves layers in declared order, then the local `.creed/` layer last. | Adds/uses the shared remote and syncs; never replaces local files. | Rejected: shared context changes go through its own review path. |

Config entries are aggregated in the manifest's declared order, joined verbatim
with `---` separators. Later layers with the same config or skill name override
earlier layers; use distinct names when both entries must be emitted. `sync`,
`diff`, `validate`, and `doctor` all use this same resolved source.

SSH authentication uses `SSH_AUTH_SOCK` or `CREED_GIT_SSH_KEY` (and optionally
`CREED_GIT_SSH_PASSPHRASE`). The Go service API also accepts an HTTPS token via
`WithGitToken`; the current CLI and MCP stdio server do not expose token
configuration, so agents using those public surfaces must use SSH for private
remotes. Tokens never belong in a remote URL, command argument, manifest, or
report.

### Target outputs

`creed list-targets` is the authoritative machine-readable output inventory.
The common target mapping is: Claude → `CLAUDE.md` + `.claude/skills/`; Codex
and generic Agents → `AGENTS.md`; Cursor → `.cursor/rules/`; Copilot →
`.github/copilot-instructions.md`; Gemini → `GEMINI.md` + `.gemini/`; OpenCode
→ `AGENTS.md` + `.opencode/agents/`; Windsurf → `.windsurfrules`; and Aider →
`.aider.conf.yml` + `CONVENTIONS.md`. A target can emit a context document,
skill directory, or target-specific config; inspect its descriptors rather than
assuming every target writes the same shape.

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
  - name: copilot
    enabled: false
    output_dir: .
  - name: cursor
    enabled: true
    output_dir: .
  - name: gemini
    enabled: false
    output_dir: .
  - name: opencode
    enabled: false
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

For organization-wide context, use an ordered layered source. Shared layers are
read first and the consumer's local `.creed/` layer is always read last:

```yaml
source:
  type: layered
  path: .creed
  layers:
    - name: org
      type: git
      remote: https://github.com/TechGodHQ/agent-context.git
      path: .creed
      ref: 0123456789abcdef0123456789abcdef01234567
```

A later layer with the same skill or config name overrides the earlier entry.
Use distinct names when both entries should be emitted. See
[`docs/layered-context-migration.md`](docs/layered-context-migration.md) for
migration and CI guidance.

## Source models

Local source is the default: Creed reads `.creed/` from the current project.
A direct git source remains supported for compatibility. Layered sharing is the
v0.4 path: the configured git layers are cloned or reused from cache, then
composed with the local source through the same SourceReader used by `sync`,
`diff`, `validate`, and `doctor`.

`creed pull <remote>` records the remote as an `org` layer and composes it; it
never replaces local `.creed/` files. `creed push` is rejected for layered
sources so shared context changes go through review instead of clobbering the
central repository.

See [`docs/layered-context-migration.md`](docs/layered-context-migration.md)
for the full manifest and migration guide.

Git remotes support public HTTPS URLs, private HTTPS URLs with the configured
service token, and SSH URLs through either `SSH_AUTH_SOCK` or an explicit
`CREED_GIT_SSH_KEY` path. If the key is passphrase-protected, set
`CREED_GIT_SSH_PASSPHRASE`. Creed passes credentials through go-git auth methods
rather than embedding tokens in clone URLs, and error messages sanitize remote
URLs before reporting auth/network failures.

Services can provide a cache directory with `WithCacheDir`; Creed stores shallow
clones under `clones/` and commit metadata under `refs/`. A cached clone is reused
only when the remote HEAD still matches the cached SHA; stale clones are removed
and refreshed automatically. Call `InvalidateCache` on the git-remote source when
a user explicitly requests a cache refresh.

## Current sync behavior

Creed uses target output descriptors to decide what each target receives. Each
known target declares output paths with semantic kinds and formats; the sync
engine renders those descriptors instead of inferring behavior from filenames.

- Context outputs receive concatenated config content, such as `CLAUDE.md`,
  `AGENTS.md`, `GEMINI.md`, `.windsurfrules`, or Aider's `CONVENTIONS.md`.
- Skill directory outputs receive one file per skill, such as `.claude/skills/`
  `.cursor/rules/`, and `.gemini/`.
- **Directory-shaped skills.** A skill entry may point at a directory that
  contains `SKILL.md` instead of a single flat file. The whole directory —
  `references/`, `templates/`, `scripts/`, `assets/`, any regular files — is
  emitted under `<skill-dir>/<name>/`, so Hermes-style skills with support
  files sync with zero data loss:

  ```yaml
  skills:
    - name: techgodhq
      path: skills/techgodhq   # directory containing SKILL.md
  ```

  emits `.claude/skills/techgodhq/SKILL.md` plus every support file, byte
  for byte. Symlinks and non-regular files inside a skill directory are
  rejected. A directory that contains only `SKILL.md` is an error — declare
  the file path directly instead.
- **Skill frontmatter validation.** When a skill file carries YAML
  frontmatter, creed validates the discovery contract: `name` must match the
  manifest entry name and `description` must be present. `validate` reports
  violations as errors naming the file and the problem; `sync` refuses to
  render a skill that breaks the contract. Skills without frontmatter are
  legal but produce a `missing_skill_frontmatter` warning, because
  downstream tools (Claude Code, Hermes) discover skills through these
  fields.
- `validate` also warns when declared skills have no enabled target with a
  skill output (for example a skills-only source with just the `agents`
  target enabled) — previously that combination was a silent no-op.
- Target-specific config outputs are rendered by explicit per-target renderers.
  Aider receives `.aider.conf.yml` pointing Aider at `CONVENTIONS.md`, plus the
  separate `CONVENTIONS.md` context file.
- `list-targets` exposes both legacy emit paths and structured descriptors so
  agents can inspect target behavior programmatically.
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
- `internal/adapters/layered`: composes ordered local/git source readers.
- `internal/usecase`: the sync engine and result model.
- `internal/service`: the canonical API shared by generated CLI, MCP, and HTTP surfaces.
- `internal/codegen`: parses the service interface and emits operation descriptors plus
  surface glue.
- `cmd` and `cmd/gen`: Cobra CLI commands generated from operation descriptors.
- `internal/mcp` and `internal/mcp/gen`: MCP tool metadata, schemas, and handlers
  generated from the same descriptors.
- `internal/httpapi` and `internal/httpapi/gen`: JSON operation catalog and call routes
  generated from the same descriptors.

User-facing surfaces follow one source-of-truth flow:

```text
internal/service.Service
        ↓ go generate ./...
generated operation descriptors
        ↓
CLI commands    MCP tools    HTTP operation routes
```

To add a generated operation:

1. Add the method to `internal/service.Service` with a doc comment.
2. Use supported inputs only: `context.Context`, no input, primitive params, or a
   DTO-like `Options`/`Request` struct with JSON tags.
3. Implement the method on the service implementation and fake services used by tests.
4. Run `go generate ./...`; this refreshes `cmd/gen/`, `internal/mcp/gen/`, and
   `internal/httpapi/gen/` from the operation descriptors.
5. Add behavior tests at the service boundary or generated surface boundary as needed.
6. Run `scripts/check-generated.sh` (or `go generate ./... && git diff --exit-code`)
   before opening a PR.

The generated HTTP surface is available as an `http.Handler` with:

- `GET /v1/operations` — list the generated operation catalog.
- `POST /v1/operations/{operation}` — call an operation with JSON input and receive a
  structured success/error envelope.

The generated MCP surface can run as a stdio server:

```bash
creed mcp serve
```

MCP clients can either launch Creed from the project whose `.creed/` directory
should be read or pass an explicit project root:

```bash
creed mcp serve --root /path/to/project
```

Example Claude Desktop configuration:

```json
{
  "mcpServers": {
    "creed": {
      "command": "creed",
      "args": ["mcp", "serve", "--root", "/path/to/project"]
    }
  }
}
```

Cursor uses the same command/args shape in its MCP server configuration. The
server exposes the generated Creed tools, including `list_targets` for inspecting
available outputs and `sync` for emitting enabled target files.

See [`docs/architecture.md`](docs/architecture.md) for more detail.

## Verification

```bash
go test -race -count=1 ./...
go vet ./...
gofmt -l .
```

## License

MIT — see [LICENSE](LICENSE).
