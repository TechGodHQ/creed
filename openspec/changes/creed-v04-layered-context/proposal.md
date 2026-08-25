# Proposal: layered context sources

## Problem

Creed v0.3 treats local and git-backed context as separate operations. `sync`,
`diff`, `validate`, and `doctor` do not share a source-resolution path, and
`pull` replaces the consumer output instead of composing shared and
repository-specific context.

## Proposal

Add an ordered layered source model. A consumer manifest declares shared
`source.layers` and Creed appends its local `.creed/` layer. The composed source
is used by sync, diff, validate, doctor, list operations, and pull. Shared
layers are read first; local context is read last. Pull records metadata and
never overwrites local source files.

## Acceptance

- A manifest can compose a git org layer and local repository configs/skills.
- `creed diff` compares the composed output and returns exit status 1 on drift.
- HTTPS token and SSH-agent/key authentication remain on the existing go-git
  auth paths, with no raw credentials in errors or reports.
- A migration guide documents the central repository and CI workflow.
