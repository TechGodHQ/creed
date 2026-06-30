// Package ports defines the interfaces (ports) that adapters implement.
// Ports are consumed by the use-case layer and keep the domain decoupled
// from infrastructure concerns (filesystem, git, network).
package ports

import (
	"context"

	"github.com/techgodhq/creed/internal/domain"
)

// SourceReader is the port for reading creed source data (manifest, skills, configs).
// Implementations MUST NOT expose filesystem paths, git internals, or HTTP details
// to callers — only the domain-level abstractions defined here.
type SourceReader interface {
	// ReadManifest reads and parses the project manifest.
	ReadManifest(ctx context.Context) (*domain.Manifest, error)

	// ReadSkill reads the full content of a skill by name.
	ReadSkill(ctx context.Context, name string) (*domain.Skill, error)

	// ListSkills returns lightweight info for all skills in the source.
	ListSkills(ctx context.Context) ([]domain.SkillInfo, error)

	// ReadConfig reads the full content of a config file by name.
	ReadConfig(ctx context.Context, name string) (*domain.ConfigFile, error)

	// ListConfigs returns lightweight info for all configs in the source.
	ListConfigs(ctx context.Context) ([]domain.ConfigInfo, error)
}
