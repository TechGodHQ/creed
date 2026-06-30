// Package localfs implements the LocalFS source adapter for reading creed
// data from a local filesystem directory.
package localfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/techgodhq/creed/internal/domain"
)

// manifestFile is the internal YAML representation of manifest.yaml.
// Field names use yaml tags to map cleanly to the on-disk format.
type manifestFile struct {
	Version int                  `yaml:"version"`
	Source  sourceConfigYAML     `yaml:"source"`
	Targets []targetConfigYAML   `yaml:"targets"`
	Skills  []domain.SkillEntry  `yaml:"skills"`
	Configs []domain.ConfigEntry `yaml:"config"`
}

type sourceConfigYAML struct {
	Type   string `yaml:"type"`
	Path   string `yaml:"path"`
	Remote string `yaml:"remote"`
}

type targetConfigYAML struct {
	Name      string `yaml:"name"`
	Enabled   bool   `yaml:"enabled"`
	OutputDir string `yaml:"output_dir"`
}

// Source reads creed data from a local filesystem directory.
// It implements ports.SourceReader.
type Source struct {
	// root is the project root directory containing the .creed/ folder.
	root string
	// creedDir is the absolute path to the .creed/ directory.
	creedDir string
}

// NewSource creates a LocalFS source reader for the given project root.
// The creed directory is resolved as root + "/" + sourcePath (default ".creed").
func NewSource(root string) *Source {
	return &Source{
		root:     root,
		creedDir: filepath.Join(root, ".creed"),
	}
}

// newSourceWithDir creates a LocalFS source reader with an explicit creed directory.
// Used internally and by GitRemote to read from a cloned repo.
func newSourceWithDir(root, creedDir string) *Source {
	return &Source{
		root:     root,
		creedDir: creedDir,
	}
}

// ReadManifest reads and parses the manifest.yaml from the .creed/ directory.
func (s *Source) ReadManifest(ctx context.Context) (*domain.Manifest, error) {
	manifestPath := filepath.Join(s.creedDir, "manifest.yaml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("manifest not found at %s", manifestPath)
		}
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var mf manifestFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	m := &domain.Manifest{
		Version: mf.Version,
		Source: domain.SourceConfig{
			Type:   mf.Source.Type,
			Path:   mf.Source.Path,
			Remote: mf.Source.Remote,
		},
		Skills:  mf.Skills,
		Configs: mf.Configs,
	}

	for _, tc := range mf.Targets {
		m.Targets = append(m.Targets, domain.TargetConfig{
			Name:      tc.Name,
			Enabled:   tc.Enabled,
			OutputDir: tc.OutputDir,
		})
	}

	// Apply default version if unset.
	if m.Version == 0 {
		m.Version = 1
	}

	return m, nil
}

// ReadSkill reads a skill's full content by name.
func (s *Source) ReadSkill(ctx context.Context, name string) (*domain.Skill, error) {
	manifest, err := s.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}

	for _, entry := range manifest.Skills {
		if entry.Name == name {
			skillPath := filepath.Join(s.creedDir, entry.Path)
			content, err := os.ReadFile(skillPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read skill file %s: %w", skillPath, err)
			}
			return &domain.Skill{
				Name:    entry.Name,
				Path:    entry.Path,
				Content: content,
			}, nil
		}
	}

	return nil, fmt.Errorf("skill not found: %s", name)
}

// ListSkills returns lightweight info for all skills declared in the manifest.
func (s *Source) ListSkills(ctx context.Context) ([]domain.SkillInfo, error) {
	manifest, err := s.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}

	skills := make([]domain.SkillInfo, 0, len(manifest.Skills))
	for _, entry := range manifest.Skills {
		skills = append(skills, domain.SkillInfo{
			Name: entry.Name,
			Path: entry.Path,
		})
	}
	return skills, nil
}

// ReadConfig reads a config file's full content by name.
func (s *Source) ReadConfig(ctx context.Context, name string) (*domain.ConfigFile, error) {
	manifest, err := s.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}

	for _, entry := range manifest.Configs {
		if entry.Name == name {
			configPath := filepath.Join(s.creedDir, entry.Path)
			content, err := os.ReadFile(configPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
			}
			return &domain.ConfigFile{
				Name:    entry.Name,
				Path:    entry.Path,
				Content: content,
			}, nil
		}
	}

	return nil, fmt.Errorf("config not found: %s", name)
}

// ListConfigs returns lightweight info for all configs declared in the manifest.
func (s *Source) ListConfigs(ctx context.Context) ([]domain.ConfigInfo, error) {
	manifest, err := s.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}

	configs := make([]domain.ConfigInfo, 0, len(manifest.Configs))
	for _, entry := range manifest.Configs {
		configs = append(configs, domain.ConfigInfo{
			Name: entry.Name,
			Path: entry.Path,
		})
	}
	return configs, nil
}
