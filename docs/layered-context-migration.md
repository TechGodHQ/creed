# Layered context migration

Creed v0.4 can compose a shared organization context repository with the
repository's own `.creed/` files. The shared layer is emitted first and the
local repository layer is emitted second, using the same `---` separator that
Creed uses for ordinary config aggregation.

## Manifest contract

Add an ordered `source.layers` list to the consumer repository's manifest:

```yaml
version: 1
source:
  type: layered
  path: .creed
  layers:
    - name: org
      type: git
      remote: https://github.com/TechGodHQ/agent-context.git
      path: .creed
      # Prefer a full commit SHA for reproducible CI.
      ref: 0123456789abcdef0123456789abcdef01234567

targets:
  - name: codex
    enabled: true
    output_dir: .
config:
  - name: repo
    path: config/repo.md
skills:
  - name: repo-review
    path: skills/repo-review.md
```

`source.layers` is ordered. Creed always appends the consumer's local source
as the final layer. A layer can be `type: local` for a second local source, or
`type: git` for a cloned source. `path` is relative to the layer root and
defaults to `.creed`. `ref` may pin a branch, tag reference, or commit SHA.

The consumer's local `source.path` remains `.creed`; custom `path` values are
for git or secondary local layers. Pull rejects URLs with embedded credentials,
queries, or fragments—configure HTTPS tokens or SSH authentication separately.

If two layers declare the same skill or config name, the later layer wins. Use
distinct names when both pieces of context should be emitted; distinct config
names are normally preferable for organization rules and repository rules.

## Migration steps

1. Create a central repository containing organization-wide config and skills
   under `.creed/`, with its own `manifest.yaml`.
2. Remove duplicated organization entries from each consumer repository only
   after the central repository has been pushed and its commit SHA recorded.
3. Add the layered `source` block above to each consumer manifest, retaining
   the consumer's targets and repository-specific entries.
4. Run `creed validate`. It fetches every configured layer and checks the
   referenced remote files as well as local files.
5. Run `creed sync` and review the generated target files.
6. Add `creed diff` to CI. It uses the same composed source and exits `1` when
   generated output drifts, so central-context changes are gated too.

## Pull behavior

`creed pull <remote>` now records the remote as an `org` layer and composes it
with the local source. It never replaces local `.creed/config/*` or
`.creed/skills/*`. If the consumer has no manifest yet, pull creates a minimal
layered manifest so `validate`, `doctor`, and `diff` remain usable afterward.

`creed push` is intentionally rejected for layered sources. Shared context
should be changed in the central repository through the normal review/PR path;
blindly copying a consumer `.creed/` directory back to the organization
repository would reintroduce the v0.3 clobbering failure mode.

## Authentication and caching

- Public and private HTTPS remotes use go-git HTTPS authentication. Configure
  the service token with the existing `WithGitToken` integration path; tokens
  are not written into URLs or reports.
- SSH remotes use `SSH_AUTH_SOCK`, or `CREED_GIT_SSH_KEY` plus
  `CREED_GIT_SSH_PASSPHRASE` for an explicit key.
- `WithCacheDir` enables commit-aware clone caching. A pinned commit reuses its
  cached clone; an unpinned branch is refreshed when its remote HEAD changes.
- `creed doctor` reports the configured remote with embedded passwords removed.

For CI-secret-backed end-to-end verification, set
`CREED_RUN_GITHUB_AUTH_INTEGRATION=1` together with
`CREED_GITHUB_HTTPS_REMOTE`/`CREED_GITHUB_HTTPS_TOKEN` and/or
`CREED_GITHUB_SSH_REMOTE`. The integration test exercises the complete layered
service path and never prints or stores the token. SSH mode uses the runner's
`SSH_AUTH_SOCK` or `CREED_GIT_SSH_KEY` configuration.
