# Development Instructions

## Commands

Run these before handing work back:

```bash
go build ./...
go test -race -count=1 ./...
go vet ./...
gofmt -l .
```

If code generation is touched, also run:

```bash
go generate ./...
go test -race -count=1 ./...
```

## Style

- Keep package boundaries clean: domain types must not import adapters or use cases.
- Ports live in `internal/ports`; adapters implement ports without leaking filesystem/git details into use cases.
- Prefer small interfaces and explicit DTOs over maply-typed blobs.
- Public exported Go identifiers need doc comments.
- Tests should cover real behavior, not just compile-time existence.
- Preserve deterministic output ordering for generated/synced files.

## Git / PR Rules

- Commits should use Shiv's global git identity so GitHub verification works.
- Runner-generated commits may include `Co-authored-by: Archon <archon@purelymail.com>` for attribution.
- Do not merge PRs automatically; human review/merge is required.

## OpenSpec

OpenSpec CLI is not installed on this machine. Edit files directly under `openspec/changes/<change>/` when creating or updating specs.

For meaningful changes:

1. Add or update `proposal.md`, `design.md`, `tasks.md`, and `specs/**/spec.md`.
2. Keep implementation PRs small and dependency-ordered.
3. Update `tasks.md` as implementation lands.
4. Do not rewrite an approved proposal during execution; log deviations elsewhere.
