# Tasks — improve-dogfood-ux

## Goal

Turn the first dogfood findings into a release-quality first-run experience: target-aware output mapping, honest dry-run reporting, useful init scaffolding, and regression coverage.

## Task order rule

Domain descriptors first, then use-case rendering, then result/CLI reporting, then init scaffolding, then regression fixtures/docs.

## Tasks

### 1) Target output descriptors (`target-output-mapping`)

- [x] **T1: Add semantic target output types** *(~1h)*
  - **Files:** `internal/domain/targets.go`, `internal/domain/types.go`, tests
  - **Do:** add `OutputKind` and `TargetOutput` types; add descriptor-returning method/function for targets.
  - **Verify:** unit tests assert `claude`, `cursor`, `codex`, and `aider` expose expected output descriptors.

- [x] **T2: Migrate target registry to output descriptors** *(~1h 30m)*
  - **Files:** `internal/domain/targets.go`, related tests
  - **Do:** define outputs for all current targets (`claude`, `cursor`, `codex`, `agents`, `windsurf`, `aider`) with explicit kinds.
  - **Verify:** existing target tests still pass; new tests cover Aider's separate `.aider.conf.yml` and `CONVENTIONS.md` descriptors.

- [x] **T3: Preserve compatibility helper for old emit-path callers** *(~45m)*
  - **Files:** `internal/domain/targets.go`, `internal/adapters/localfs/emitter.go` if needed
  - **Do:** keep `EmitPaths` behavior or provide a compatibility adapter until all call sites migrate.
  - **Verify:** `go test ./internal/domain ./internal/adapters/localfs`.

### 2) Descriptor-aware rendering (`target-output-mapping`)

- [x] **T4: Replace bare-path prepareFiles with descriptor-aware rendering** *(~2h)*
  - **Files:** `internal/usecase/sync.go`, tests
  - **Do:** render context outputs from config entries, skill directory outputs from skills, and config outputs from target-specific config renderers.
  - **Verify:** unit tests cover `claude`, `codex`, `cursor`, and `aider` candidate files.

- [x] **T5: Add minimal Aider config rendering** *(~1h 30m)*
  - **Files:** `internal/usecase/sync.go`, tests
  - **Do:** emit `.aider.conf.yml` with minimal deterministic YAML and `CONVENTIONS.md` from context/config content.
  - **Verify:** test asserts both files exist and contain appropriate content.

- [x] **T6: Preserve deterministic ordering** *(~45m)*
  - **Files:** `internal/usecase/sync.go`, tests
  - **Do:** ensure emitted file candidate order is stable across repeated generation.
  - **Verify:** repeated candidate generation test compares path order exactly.

### 3) Dry-run reporting (`dry-run-reporting`)

- [x] **T7: Add would-write counts to result model** *(~45m)*
  - **Files:** `internal/usecase/result.go`, `internal/usecase/sync.go`, tests
  - **Do:** add `FilesWouldWrite` or equivalent and increment it for dry-run candidates that would write.
  - **Verify:** dry-run unit tests assert would-write and written counts separately.

- [x] **T8: Fix CLI dry-run summary output** *(~45m)*
  - **Files:** `cmd/sync.go`, command tests
  - **Do:** print `would_write` counts in dry-run summaries and keep normal summaries unchanged.
  - **Verify:** CLI test covers dry-run summary with would-write files.

### 4) Init scaffolding (`init-scaffolding`)

- [x] **T9: Upgrade `creed init` scaffold files** *(~1h 30m)*
  - **Files:** `cmd/init.go`, `internal/service/impl.go`, tests
  - **Do:** create `.creed/config/project.md`, `.creed/config/development.md`, and `.creed/skills/review.md` in addition to manifest.
  - **Verify:** temp-dir init test asserts all scaffold files exist.

- [x] **T10: Enable practical default targets in generated manifest** *(~45m)*
  - **Files:** init/service manifest code, tests
  - **Do:** generated manifest enables `claude`, `codex`, and `cursor` with `output_dir: .`.
  - **Verify:** test reads manifest and asserts defaults.

- [x] **T11: Make init non-destructive** *(~1h)*
  - **Files:** init/service scaffold code, tests
  - **Do:** skip existing files rather than overwriting; return/report skipped files.
  - **Verify:** rerun init after editing scaffold files and assert custom content is preserved.

### 5) Dogfood regression (`dogfood-regression`)

- [x] **T12: Add reduced dogfood fixture** *(~45m)*
  - **Files:** `testdata/dogfood-creed/` or `internal/integration/testdata/dogfood-creed/`
  - **Do:** create fixture manifest/config/skills covering `claude`, `codex`, `cursor`, and `aider`.
  - **Verify:** fixture is consumed by integration tests, not just checked in.

- [x] **T13: Add dogfood integration test** *(~2h)*
  - **Files:** `internal/integration/dogfood_test.go`
  - **Do:** sync fixture to temp output and assert expected files/content for AGENTS, CLAUDE, Cursor rules, and Aider.
  - **Verify:** test passes under `go test -race -count=1 ./internal/integration`.

- [x] **T14: Add idempotency assertion to dogfood regression** *(~45m)*
  - **Files:** `internal/integration/dogfood_test.go`
  - **Do:** run sync twice and assert second run skips unchanged files and content remains stable.
  - **Verify:** integration test covers repeated sync.

### 6) Docs and delivery

- [ ] **T15: Update README for improved init and dry-run behavior** *(~45m)*
  - **Files:** `README.md`
  - **Do:** document scaffolded files, default targets, dry-run summary semantics, and Aider behavior.
  - **Verify:** examples match current CLI output.

- [ ] **T16: Run full verification and mark tasks complete** *(~45m)*
  - **Do:** execute:
    - `go build ./...`
    - `go test -race -count=1 ./...`
    - `go vet ./...`
    - `gofmt -l .`
  - **Verify:** all checks pass; task checkboxes reflect implemented work.
