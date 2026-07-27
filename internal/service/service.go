// Package service defines Creed's canonical application API.
//
// The Service interface is the single contract that generated interaction
// surfaces (CLI, MCP, and HTTP) wrap. Keeping all user-visible operations here
// prevents surface drift as capabilities grow.
package service

import (
	"context"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/usecase"
)

// Service is the canonical Creed API surface shared by CLI, MCP, and HTTP
// wrappers.
type Service interface {
	// Init bootstraps a Creed project at the service root.
	Init(ctx context.Context, projectName string) error
	// Sync syncs configured Creed context to one or more targets.
	Sync(ctx context.Context, opts usecase.SyncOptions) (*usecase.SyncResult, error)
	// Validate checks the manifest and its referenced Creed source files without
	// writing outputs. Validation errors are returned in the result so generated
	// CLI, MCP, and HTTP callers receive the same structured diagnostics.
	Validate(ctx context.Context) (ValidationResult, error)
	// AddSkill registers a skill file in the manifest.
	AddSkill(ctx context.Context, name, sourcePath string) error
	// RemoveSkill removes a skill registration from the manifest.
	RemoveSkill(ctx context.Context, name string) error
	// ListSkills returns all registered skills.
	ListSkills(ctx context.Context) ([]domain.SkillInfo, error)
	// AddConfig registers a configuration file in the manifest.
	AddConfig(ctx context.Context, name, sourcePath string) error
	// RemoveConfig removes a configuration file registration from the manifest.
	RemoveConfig(ctx context.Context, name string) error
	// ListConfigs returns all registered configuration files.
	ListConfigs(ctx context.Context) ([]domain.ConfigInfo, error)
	// ListTargets returns all known targets with manifest enablement metadata.
	ListTargets(ctx context.Context) ([]domain.TargetInfo, error)
	// EnableTarget enables a target in the manifest, creating it if needed.
	EnableTarget(ctx context.Context, name string) error
	// DisableTarget disables a target in the manifest, creating it if needed.
	DisableTarget(ctx context.Context, name string) error
	// Pull syncs from a git remote source into the service root.
	Pull(ctx context.Context, remoteURL string) error
	// Push publishes local source changes to the configured remote.
	Push(ctx context.Context, remoteURL string) error
	// Watch registers a Watcher on the project's canonical .creed/
	// sources and runs a debounced sync for each stable change until
	// ctx is cancelled. This is a blocking operation; callers must
	// provide a cancellable context.
	Watch(ctx context.Context, opts usecase.WatchOptions, sink usecase.WatchSink) error
	// Doctor produces a diagnostic report covering the project root,
	// manifest and source presence, validation summary, configured
	// targets, and git availability. It is non-mutating and never
	// exposes sensitive values. Generated CLI, MCP, and HTTP callers
	// receive the same structured report.
	Doctor(ctx context.Context) (DoctorReport, error)
}
