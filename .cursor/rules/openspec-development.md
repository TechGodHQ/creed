# OpenSpec Development Skill

Use this when adding or changing significant Creed behavior.

## Workflow

1. Create a change directory under `openspec/changes/<change-name>/`.
2. Write:
   - `proposal.md`: what problem this solves and what changes.
   - `design.md`: architecture, alternatives, risks, dependency choices.
   - `tasks.md`: small implementation tasks with verification commands.
   - `specs/<capability>/spec.md`: SHALL-style requirements.
3. Split Linear issues and PRs by dependency boundaries, not by arbitrary file count.
4. Keep each PR independently reviewable and testable.
5. Mark tasks `[x]` only when code is merged and verified.

## Local Constraint

The `openspec` CLI is not available in this environment. Validate by reading the artifacts directly and by running the repository test/build commands.

## Good Task Shape

Each task should include:

- Files or packages touched.
- Behavior being added.
- Acceptance criteria.
- Exact verification commands.
- Dependencies on prior tasks if any.
