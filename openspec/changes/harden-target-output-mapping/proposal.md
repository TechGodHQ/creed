# Proposal — harden-target-output-mapping

## Why

Creed already has first-pass target output descriptors and descriptor-aware rendering, but the contract is still too thin for release-quality target support:

- output descriptors are not exposed well enough through service/CLI/MCP surfaces
- target-specific rendering is still centralized in sync orchestration instead of owned by explicit renderers
- Aider support is minimal and easy to regress
- docs/context still describe target mapping as a known limitation even after the first-pass implementation
- there is no dedicated clean-tree guard proving real target output behavior stays stable across dogfood syncs

This change makes target-output-mapping a durable product surface rather than an internal implementation detail.

## What changes

- Extend the target-output-mapping contract with named/inspectable output descriptors.
- Move target-specific rendering decisions behind explicit renderer functions/registry rather than ad-hoc branches in sync orchestration.
- Expose descriptor details through list-target surfaces so users and agents can inspect what Creed will emit before syncing.
- Add regression coverage for all supported targets, with special focus on Aider's multi-file semantics.
- Update docs and generated context to remove stale limitations and explain the mapping model.

## Capabilities

- `target-output-mapping`

## Impact

- Touches domain target descriptor DTOs and tests.
- Touches sync rendering internals and integration tests.
- Touches service/CLI/MCP list-target output shape.
- Touches README and dogfooded context files.
- Should preserve backward compatibility for existing manifests and current emitted paths.
