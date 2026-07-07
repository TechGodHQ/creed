# Proposal — improve-dogfood-ux

## Why

Dogfooding Creed on the Creed repository proved the core sync loop works, but exposed three user-facing gaps before v0.1: target outputs are too generic, dry-run summaries are misleading, and `creed init` leaves too much manual setup. A tool whose whole pitch is “one source, many agent contexts” needs the first-run path to feel obvious, not like a little gremlin asking the user to hand-author its skeleton.

## What Changes

- Add target-aware output mapping so targets can route different source entries to specific generated files instead of always aggregating configs into the first file path.
- Improve dry-run reporting so summary counts match `would_write` file statuses.
- Upgrade `creed init` to scaffold a useful `.creed/` tree with starter config, skill files, and enabled default targets.
- Add a dogfood regression fixture that exercises Creed-like source context and verifies emitted files.

## Capabilities

- `target-output-mapping`: target-specific file roles and deterministic mapping from source entries to output paths.
- `dry-run-reporting`: accurate dry-run result counts and clearer CLI output.
- `init-scaffolding`: useful project bootstrap for `.creed/` source context.
- `dogfood-regression`: regression coverage for self-hosted Creed context workflows.

## Non-goals

- No code-generated CLI/MCP work in this change.
- No hosted sync service or daemon.
- No migration of existing user manifests beyond backward-compatible defaults.
- No full templating language for generated files.

## Impact

This change makes Creed usable as an actual first-run tool: initialize, edit obvious files, dry-run, sync, and trust the output. It also provides the product feedback loop needed before release and before investing in service-interface code generation.
