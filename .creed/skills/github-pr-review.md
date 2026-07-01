# GitHub PR Review Skill

Use this when reviewing a Creed pull request.

## Checklist

1. Read the issue/spec goal first. Do not review a diff in isolation.
2. Confirm the change actually satisfies the stated goal.
3. Check package boundaries:
   - `internal/domain` stays dependency-light.
   - `internal/usecase` depends on ports/domain, not concrete adapters.
   - adapters do not import service or command packages.
4. Run or verify:
   - `go build ./...`
   - `go test -race -count=1 ./...`
   - `go vet ./...`
   - `gofmt -l .`
5. Look for user-visible regressions in CLI flags, output text, and manifest schema.
6. Check generated/synced output determinism if the change touches sync/codegen paths.

## Review Bias

Prefer correctness and product behavior over style nits. Small style comments are only worth raising if they prevent future bugs or API confusion.
