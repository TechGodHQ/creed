# Changelog

All notable changes to creed are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] — pending publication

### Added

- **Directory-shaped skills.** A skill can be a directory containing `SKILL.md`
  and support files; Creed preserves the full tree, validates available
  frontmatter, and reports when no enabled target can emit skills.
- **Drift checks and discovery.** `creed check` is a CI gate with a stable diff;
  `creed diff` remains a non-gating preview. `list-targets` and `list-skills`
  work outside a project, and `doctor` detects generated-output drift.
- **Generated-output attribution.** Sync manages target-owned
  `linguist-generated` attributes while preserving user-authored rules.

### Changed

- **Safer pulls.** `creed pull` supports `--dry-run` and `--force` across the
  generated CLI, MCP, and HTTP surfaces. It previews changes and protects
  existing emitted files unless force is explicitly requested.
- The CLI and MCP implementation now report version `0.5.0`.

### Known limitations

- The `agents` and `codex` targets emit `AGENTS.md` but do not yet emit skill
  files; directory skills require a target with skill-directory output.

### Development and maintenance

- Dogfood Creed's layered organization-context source and keep its generated
  agent guidance in sync through CI.

## [0.4.1] — 2026-08-27

Corrective release for the layered-context feature shipped in v0.4.0. The
published v0.4.0 artifact incorrectly reported its CLI and MCP implementation
version as 0.3.0. That immutable tag remains unchanged; install v0.4.1 for the
layered-context release.

### Fixed

- CLI `--version` and the MCP server implementation version now report
  `0.4.1`.

### Added

- **Layered context sources.** Compose cached git organization context with
  per-repository `.creed/` files; `sync`, `diff`, `validate`, and `doctor` now
  share the same resolved source path.
- **Layered migration guide.** Document manifest configuration, pinned refs,
  authentication, CI drift gating, and non-clobbering pull/push behavior.

## [0.3.0] — 2026-08-19

The first post-reset feature release. Everything shipped since v0.1.0 lands
here: three new targets, a generated interaction surface set (CLI + MCP +
HTTP), new workflow commands, and a hardened git remote source.

### Added

- **Generated interaction surfaces.** `internal/service.Service` is now the
  single source of truth for operations; `go generate ./...` emits CLI
  commands, MCP tools, and HTTP routes from operation descriptors parsed off
  the service interface. `scripts/check-generated.sh` gates CI on generated
  code being current and idempotent.
- **MCP stdio server.** `creed mcp serve [--root PATH]` runs the generated
  tool surface over stdio for MCP clients such as Claude Desktop and Cursor.
- **New targets.** `gemini` (`GEMINI.md` + `.gemini/` skills), `copilot`
  (`.github/copilot-instructions.md`), and `opencode`
  (`.opencode/agents/` + AGENTS.md) join the target registry.
- **`creed watch`** — debounced auto-sync when `.creed/` sources change,
  with `--target` and `--debounce` flags and `--quiet` mode.
- **`creed diff`** — line-level preview of changes between rendered target
  output and what is on disk.
- **`creed validate`** — manifest + referenced source validation without
  writing outputs; structured diagnostics surfaced identically across CLI,
  MCP, and HTTP.
- **`creed doctor`** — non-mutating diagnostic report covering project root,
  manifest presence, validation summary, configured targets, and git
  availability. Never exposes sensitive values.
- **Config management operations.** `add-config`, `remove-config`,
  `list-configs`, `add-skill`, `remove-skill`, `list-skills`,
  `enable-target`, `disable-target` — manifest editing without hand-editing
  YAML.

### Changed

- **Git remote source hardening.** Credentials now flow through go-git auth
  methods instead of embedded clone URLs; remote URLs are sanitized in error
  messages. Private HTTPS (service token), public HTTPS, and SSH
  (`SSH_AUTH_SOCK` or `CREED_GIT_SSH_KEY` + `CREED_GIT_SSH_PASSPHRASE`)
  are supported. Cached clones are reused only when remote HEAD matches the
  cached SHA.
- CLI `--version` and the MCP server implementation version now report
  `0.3.0`.

## [0.1.0] — 2026-07-14

First release. Local + git-remote sources, target output descriptors,
sync/dry-run/force, and the ports-and-adapters core.

## [0.2.0] — 2026-07-07 (pre-reset, superseded)

Tagged before the repository reset, on an ancestor of the v0.1.0 commit.
The module proxy and checksum database pin `v0.2.0` to that pre-reset tree,
so the version number is permanently associated with it. **The 0.3.0 release
above supersedes everything in it.** Do not install v0.2.0; its release
notes are preserved on GitHub only for historical reference.

[0.5.0]: https://github.com/techgodhq/creed/releases/tag/v0.5.0
[0.4.1]: https://github.com/techgodhq/creed/releases/tag/v0.4.1
[0.3.0]: https://github.com/techgodhq/creed/releases/tag/v0.3.0
[0.1.0]: https://github.com/techgodhq/creed/releases/tag/v0.1.0
[0.2.0]: https://github.com/techgodhq/creed/releases/tag/v0.2.0
