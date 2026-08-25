# Design: layered context sources

## Manifest

`domain.SourceConfig` gains `Ref` and ordered `Layers`. Each layer has a name,
backend type, source path, remote URL, and optional ref. The parser accepts
`source.layers` as the canonical spelling and `source.overlays` as a compatible
alias; serializers write `layers`.

`local`, `git`, and `layered` source types remain supported. A local source with
layers is treated as layered for compatibility. A direct git source remains a
single-layer legacy mode.

## Composition

`internal/adapters/layered.Source` implements `ports.SourceReader` and wraps
local or git readers. It merges manifests deterministically, with the last
entry of a duplicate skill/config name replacing the earlier entry. Targets
come from the last layer that declares them, with the consumer manifest taking
precedence when present. The service always appends the consumer local reader
last.

`internal/service/source.go` is the single source-graph factory. Sync, diff,
list, validate, doctor, watch, and pull use it, so those surfaces cannot silently
choose different source semantics. Git readers retain their cache/auth behavior
and now support a source subdirectory and optional ref pin.

## Validation and safety

Validation strictly parses the consumer manifest, validates every layer's type,
name, path, remote, and ref, then opens the resolved source graph. It reads all
remote-declared entries through the same composed reader used by sync. Local
files retain symlink, traversal, regular-file, permission, and empty-content
checks. LocalFS also rejects traversal paths before reading remote files.

`doctor` reuses `Validate` and includes sanitized layered remotes in its report.
`pull` writes only the consumer manifest metadata needed to remember the shared
layer; it refuses `push` for layered sources to prevent central-repository
clobbering.
