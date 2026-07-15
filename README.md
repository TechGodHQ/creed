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
| Gemini CLI | `GEMINI.md`, `.gemini/` |
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
`output_dir: .`. Less universal targets (`agents`, `aider`, `gemini`, and
`windsurf`) are listed but disabled until you opt in.

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
  - name: gemini
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

See [`docs/architecture.md`](docs/architecture.md) for more detail.

## Verification

```bash
go test -race -count=1 ./...
go vet ./...
gofmt -l .
```

## License

MIT — see [LICENSE](LICENSE).
