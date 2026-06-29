## Context

Creed is a Go CLI and MCP server for syncing AI context (skills, specs, config files) from a single source to multiple target tools. The current repo has a stub CLI (`cmd/init.go`, `cmd/sync.go`) and a basic target registry (`internal/target/target.go`). This change replaces the stubs with a proper ports & adapters architecture, adds git remote support, and establishes a code-generated service interface that guarantees CLI/MCP/HTTP surface parity.

## Goals / Non-Goals

**Goals:**
- Establish the full ports & adapters layering (domain → ports → adapters → use cases → surfaces)
- Define a single `Service` interface that serves as the API contract for all interaction surfaces
- Code-generate CLI (Cobra) and MCP tool registrations from the service interface
- Support local filesystem and git remote as source backends
- File-level sync: treat skills/config as opaque blobs, copy verbatim to target paths
- Idempotent sync: running twice produces identical results
- Support 6 targets: claude, cursor, codex, agents, windsurf, aider

**Non-Goals:**
- No HTTP server (interface supports it, implementation deferred)
- No content transformation or markdown parsing
- No hosted/team features, no user accounts, no web UI
- No drift detection or reconciliation heuristics (sync overwrites)
- No skill versioning or dependency resolution

## Decisions

### D1: Ports & Adapters (Hexagonal)

Domain logic has zero external dependencies. All I/O goes through port interfaces defined in `internal/ports/`. Adapters implement those ports in `internal/adapters/`.

**Why hexagonal over layered:** The core sync logic must be testable without filesystem or network access. Ports make the boundaries explicit and prevent domain pollution. This also makes the future HTTPStore adapter trivial to add — implement the same `SourceReader` interface, nothing in the domain changes.

**Alternative considered:** Simple layered architecture (CLI calls services calls repositories). Rejected because it couples domain logic to infrastructure concerns and makes future adapter additions messy.

### D2: Code-Generated Service Surfaces

A single Go `Service` interface in `internal/service/service.go` defines the complete API surface. A `go:generate` directive runs a code generator (`internal/codegen/`) that produces thin wrappers for each surface.

```
┌──────────────────────────────────────────────────┐
│              Interaction Surfaces                 │
│  ┌─────────┐  ┌───────────┐  ┌────────────────┐  │
│  │   CLI   │  │    MCP    │  │  HTTP (future)  │  │
│  │ (gen'd) │  │  (gen'd)  │  │   (gen'd)      │  │
│  └────┬────┘  └─────┬─────┘  └───────┬────────┘  │
│       └─────────────┼───────────────┘            │
│                     ▼                             │
├──────────────────────────────────────────────────┤
│              Service Interface                    │
│   internal/service/service.go                     │
│   ┌────────────────────────────────────────────┐  │
│   │  Init(projectName string) error            │  │
│   │  Sync(opts SyncOptions) (*SyncResult, err) │  │
│   │  AddSkill(name, sourcePath string) error   │  │
│   │  ListTargets() []TargetInfo                │  │
│   │  AddTarget(name string, cfg TargetConfig)  │  │
│   │  Pull(remoteURL string) error              │  │
│   │  Push(remoteURL string) error              │  │
│   └────────────────────────────────────────────┘  │
├──────────────────────────────────────────────────┤
│              Core Library                         │
│  ┌────────────────────────────────────────────┐  │
│  │         Use Cases (app layer)               │  │
│  │  SyncEngine  •  InitProject  •  Pull/Push   │  │
│  └───────────────────┬────────────────────────┘  │
│                      ▼                            │
│  ┌────────────────────────────────────────────┐  │
│  │            Ports (interfaces)               │  │
│  │  SourceReader • TargetEmitter • ManifestIO  │  │
│  └───────────────────┬────────────────────────┘  │
│                      ▼                            │
│  ┌────────────────────────────────────────────┐  │
│  │           Domain Models                     │  │
│  │  Skill • Spec • ConfigFile • Target         │  │
│  │  Manifest • SyncResult • SourceConfig       │  │
│  └────────────────────────────────────────────┘  │
├──────────────────────────────────────────────────┤
│                  Adapters                         │
│  ┌───────────┐ ┌───────────┐ ┌────────────────┐  │
│  │ LocalFS   │ │ GitRemote │ │  LocalFS       │  │
│  │ (source)  │ │ (source)  │ │  (emitter)     │  │
│  └───────────┘ └───────────┘ └────────────────┘  │
└──────────────────────────────────────────────────┘
```

**Why generate instead of manual wrappers:** Shiv's explicit requirement — no drift between CLI, MCP, and HTTP. Adding a method to `Service` should automatically produce the corresponding Cobra command and MCP tool registration. Manual wrapping guarantees drift over time.

**Generation approach:** Go reflection over the `Service` interface at build time. The generator reads method names, parameter types, and doc comments to produce:
- CLI: Cobra commands with flags mapped from params, help text from doc comments
- MCP: Tool definitions with JSON Schema derived from Go struct tags
- HTTP (future): Route handlers with request/response types

**Alternative considered:** gRPC + protoc for multi-surface generation. Rejected as overkill for a Go-only tool — adds protobuf toolchain dependency for no current benefit.

### D3: Two Source Adapters (LocalFS + GitRemote)

```
SourceReader (port)
│
├── LocalFS adapter
│   └── Reads .creed/ directory structure in the project
│       .creed/
│         manifest.yaml     # what targets, what source
│         skills/           # skill blobs (opaque)
│         specs/            # spec blobs (opaque)
│         config/           # config files (opaque)
│
├── GitRemote adapter
│   └── Clones/pulls a git repo to temp, reads like LocalFS
│      Supports: https://, git://, ssh:// URLs
│      Caches last-pulled commit hash to avoid redundant clones
│
└── HTTPStore adapter (future)
    └── Talks to a creed server instance
```

**Why git-as-backend first:** Zero infrastructure cost. Users already have GitHub/GitLab/Gitea accounts. No auth complexity beyond existing git credentials. Git gives us versioning, branching, and pull requests for free.

**Alternative considered:** S3 bucket or database backend. Rejected — adds hosting burden and auth complexity for v1.

### D4: Target Registry with Emit Path Mapping

Each target is a struct with metadata and an `EmitPaths` function. The registry maps target names to their definitions. The existing `internal/target/target.go` will be refactored into the new domain model.

```
Target {
  Name        string
  DisplayName string
  EmitPaths   func(projectName string) []string
}
```

The `projectName` parameter allows targets to generate dynamic paths (e.g., embedding the project name in file contents — future use, not v1).

### D5: Manifest Format (creed.yaml / .creed/manifest.yaml)

```yaml
version: 1
source:
  type: local          # local | git
  path: .creed         # for local: directory path
  remote: ""           # for git: clone URL (optional, for push/pull)

targets:
  - name: claude
    enabled: true
    output_dir: .      # where to emit relative to project root
  - name: cursor
    enabled: true
    output_dir: .
  - name: codex
    enabled: false

skills:
  - name: code-review
    path: skills/code-review.md
  - name: testing
    path: skills/testing.md

config:
  - name: project-context
    path: config/project.md
```

**Why YAML over TOML/JSON:** YAML supports comments (critical for config that users will tweak), is already the ecosystem standard for Go projects (k8s, CI configs), and the Go ecosystem has mature YAML libraries.

## Interface Contracts

### SourceReader (port)

```go
type SourceReader interface {
    ReadManifest(ctx context.Context) (*Manifest, error)
    ReadSkill(ctx context.Context, name string) (*Skill, error)
    ListSkills(ctx context.Context) ([]SkillInfo, error)
    ReadConfig(ctx context.Context, name string) (*ConfigFile, error)
    ListConfigs(ctx context.Context) ([]ConfigInfo, error)
}
```

### TargetEmitter (port)

```go
type TargetEmitter interface {
    Emit(ctx context.Context, target Target, files []EmittedFile) ([]EmitResult, error)
    Clean(ctx context.Context, target Target) error
}
```

### SyncEngine (use case)

```go
type SyncEngine struct {
    source  ports.SourceReader
    emitter ports.TargetEmitter
}

func (s *SyncEngine) Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error)
```

### Service (API surface)

```go
type Service interface {
    Init(ctx context.Context, projectName string) error
    Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error)
    AddSkill(ctx context.Context, name, sourcePath string) error
    RemoveSkill(ctx context.Context, name string) error
    ListSkills(ctx context.Context) ([]SkillInfo, error)
    ListTargets(ctx context.Context) ([]TargetInfo, error)
    EnableTarget(ctx context.Context, name string) error
    DisableTarget(ctx context.Context, name string) error
    Pull(ctx context.Context, remoteURL string) error
    Push(ctx context.Context, remoteURL string) error
}
```

## Dependencies

| Dependency | Purpose | Justification |
|---|---|---|
| `spf13/cobra` | CLI framework | Already in use. Standard Go CLI library. |
| `go-git/go-git` | Git remote source adapter | Pure-Go git implementation. No system git dependency. Well-maintained, used by major projects. |
| `mark3labs/mcp-go` | MCP server | Most mature Go MCP server library. Needed for MCP surface code generation. |
| `gopkg.in/yaml.v3` | Manifest parsing | Standard YAML library for Go. Already widely used. |

## Risks / Trade-offs

- **Code generation complexity** → The generator is the highest-risk component. Start simple: reflection-based, template-driven. If generation proves fragile, fall back to manual wrappers with a linter that enforces interface coverage. Either way, drift is prevented.
- **go-git auth limitations** → go-git's SSH support has quirks. Mitigation: support HTTPS with token auth first, SSH later. Document known limitations.
- **Manifest format churn** → v1 manifest format will evolve. Mitigation: include `version` field from day one, implement a version check that errors clearly on unknown versions.
- **MCP library immaturity** → MCP is a young protocol; libraries may have breaking changes. Mitigation: isolate MCP code to `internal/mcp/`, keep the generated wrapper thin so library swaps are localized.
