// Package domain defines the core vocabulary types for creed.
// These types are zero-dependency (stdlib only) and represent the
// fundamental building blocks of the sync system.
package domain

import "time"

// Skill represents an AI skill file stored in the creed source.
type Skill struct {
	// Name is the canonical skill identifier (e.g., "code-review").
	Name string
	// Path is the relative path to the skill file within the source.
	Path string
	// Content is the raw file content of the skill.
	Content []byte
}

// SkillInfo is a lightweight summary of a skill, without content payload.
// Used for listing operations.
type SkillInfo struct {
	// Name is the canonical skill identifier.
	Name string
	// Path is the relative path to the skill file within the source.
	Path string
}

// Spec represents a specification file stored in the creed source.
type Spec struct {
	// Name is the canonical spec identifier.
	Name string
	// Path is the relative path to the spec file within the source.
	Path string
	// Content is the raw file content of the spec.
	Content []byte
}

// ConfigFile represents a configuration or context file stored in the creed source.
type ConfigFile struct {
	// Name is the canonical config identifier (e.g., "project-context").
	Name string
	// Path is the relative path to the config file within the source.
	Path string
	// Content is the raw file content of the config.
	Content []byte
}

// ConfigInfo is a lightweight summary of a config file, without content payload.
// Used for listing operations.
type ConfigInfo struct {
	// Name is the canonical config identifier.
	Name string
	// Path is the relative path to the config file within the source.
	Path string
}

// Target represents an AI tool that can receive synced context files.
type Target struct {
	// Name is the canonical target name (e.g., "claude", "cursor").
	Name string
	// DisplayName is the human-readable target name.
	DisplayName string
	// Outputs returns semantic output descriptors this target expects,
	// given the project name.
	Outputs func(projectName string) []TargetOutput
	// EmitPaths returns the relative file paths this target expects,
	// given the project name. For example: ["CLAUDE.md", ".claude/skills/"].
	//
	// Use Outputs for new semantic target mappings. EmitPaths is kept as a
	// compatibility helper for existing emit-path callers.
	EmitPaths func(projectName string) []string
}

// OutputKind describes the semantic role of a target output path.
type OutputKind string

const (
	// OutputKindContext receives aggregated project context/config content.
	OutputKindContext OutputKind = "context"
	// OutputKindSkillDir receives one emitted skill file per source skill.
	OutputKindSkillDir OutputKind = "skill_dir"
	// OutputKindConfig receives target-specific generated configuration.
	OutputKindConfig OutputKind = "config"
)

// TargetOutput describes a semantic output path for a target.
type TargetOutput struct {
	// Path is the relative file or directory path to emit.
	Path string
	// Kind is the semantic role this output plays for the target.
	Kind OutputKind
	// Format is an advisory content format label, such as markdown or yaml.
	Format string
}

// TargetInfo is a lightweight summary of a target, used for listing and display.
type TargetInfo struct {
	// Name is the canonical target name.
	Name string
	// DisplayName is the human-readable target name.
	DisplayName string
	// Enabled indicates whether the target is active in the manifest.
	Enabled bool
	// OutputDir is the directory relative to project root where files are emitted.
	OutputDir string
	// EmitPaths lists the file paths this target expects.
	//
	// Deprecated: use Outputs for structured descriptor data. EmitPaths is kept
	// for compatibility with existing callers and human-oriented displays.
	EmitPaths []string
	// Outputs lists the structured output descriptors this target expects.
	Outputs []TargetOutput
}

// SourceConfig configures the source backend for reading creed data.
type SourceConfig struct {
	// Type is the source backend type: "local" or "git".
	Type string
	// Path is the directory path (for local source, typically ".creed").
	Path string
	// Remote is the git clone URL (for git source). Empty for local.
	Remote string
}

// TargetConfig represents a target entry in the manifest.
type TargetConfig struct {
	// Name is the canonical target name.
	Name string
	// Enabled indicates whether this target should receive synced files.
	Enabled bool
	// OutputDir is the directory relative to project root where files are emitted.
	OutputDir string
}

// SkillEntry represents a skill entry in the manifest.
type SkillEntry struct {
	// Name is the canonical skill identifier.
	Name string
	// Path is the relative path to the skill file within the source directory.
	Path string
}

// ConfigEntry represents a config entry in the manifest.
type ConfigEntry struct {
	// Name is the canonical config identifier.
	Name string
	// Path is the relative path to the config file within the source directory.
	Path string
}

// Manifest is the project configuration, stored as .creed/manifest.yaml.
type Manifest struct {
	// Version is the manifest format version. Defaults to 1 for new manifests.
	Version int
	// Source configures the source backend.
	Source SourceConfig
	// Targets lists the configured targets.
	Targets []TargetConfig
	// Skills lists the skill entries.
	Skills []SkillEntry
	// Configs lists the config entries.
	Configs []ConfigEntry
}

// SyncResult captures the outcome of a sync operation for a single target.
type SyncResult struct {
	// Target is the canonical target name.
	Target string
	// FilesWritten is the number of files written or updated.
	FilesWritten int
	// FilesSkipped is the number of files that were already up-to-date.
	FilesSkipped int
	// Duration is the time taken for this target's sync.
	Duration time.Duration
	// Error holds any error that occurred during sync, or nil on success.
	Error error
}

// NewManifest creates a Manifest with sensible defaults.
// Version defaults to 1, source defaults to local type.
func NewManifest() *Manifest {
	return &Manifest{
		Version: 1,
		Source: SourceConfig{
			Type: "local",
			Path: ".creed",
		},
	}
}
