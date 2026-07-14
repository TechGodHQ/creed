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
	// AddSkill registers a skill file in the manifest.
	AddSkill(ctx context.Context, name, sourcePath string) error
	// RemoveSkill removes a skill registration from the manifest.
	RemoveSkill(ctx context.Context, name string) error
	// ListSkills returns all registered skills.
	ListSkills(ctx context.Context) ([]domain.SkillInfo, error)
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
}
