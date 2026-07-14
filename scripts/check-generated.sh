#!/usr/bin/env bash
set -euo pipefail

if [[ -n "$(git status --porcelain)" ]]; then
  echo "generated-code check requires a clean working tree before running go generate" >&2
  git status --short >&2
  exit 1
fi

go generate ./...

if ! git diff --quiet --exit-code; then
  echo "go generate changed tracked files; run go generate ./... and commit the results" >&2
  git diff --name-only >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "go generate created untracked files; commit or ignore the generated outputs" >&2
  git status --short >&2
  exit 1
fi

echo "generated code is current"
