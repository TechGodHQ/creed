// Package layered composes ordered Creed source readers into one deterministic
// source. It keeps infrastructure-specific readers behind the SourceReader port.
package layered

import (
	"context"
	"fmt"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

// Source reads several source layers as one composed SourceReader. Layers are
// ordered from shared context to repository context; later entries with the
// same name replace earlier entries while retaining deterministic order.
type Source struct {
	layers []ports.SourceReader
}

var _ ports.SourceReader = (*Source)(nil)

// NewSource creates a layered source from one or more ordered readers.
func NewSource(layers ...ports.SourceReader) *Source {
	copied := append([]ports.SourceReader(nil), layers...)
	return &Source{layers: copied}
}

type snapshot struct {
	manifests []*domain.Manifest
}

func (s *Source) readSnapshot(ctx context.Context) (*snapshot, error) {
	if len(s.layers) == 0 {
		return nil, fmt.Errorf("layered source requires at least one layer")
	}
	result := &snapshot{manifests: make([]*domain.Manifest, 0, len(s.layers))}
	for i, layer := range s.layers {
		if layer == nil {
			return nil, fmt.Errorf("layered source layer %d is nil", i)
		}
		manifest, err := layer.ReadManifest(ctx)
		if err != nil {
			return nil, fmt.Errorf("read layer %d manifest: %w", i, err)
		}
		if manifest == nil {
			return nil, fmt.Errorf("layer %d returned a nil manifest", i)
		}
		result.manifests = append(result.manifests, manifest)
	}
	return result, nil
}

// ReadManifest returns the composed manifest. Targets come from the last
// layer that declares them, while skills and configs are merged in layer order.
func (s *Source) ReadManifest(ctx context.Context) (*domain.Manifest, error) {
	state, err := s.readSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	last := state.manifests[len(state.manifests)-1]
	combined := &domain.Manifest{
		Version: last.Version,
		Source: domain.SourceConfig{
			Type: "layered",
			Path: last.Source.Path,
		},
		Skills:  mergeSkills(state.manifests),
		Configs: mergeConfigs(state.manifests),
	}
	for i := len(state.manifests) - 1; i >= 0; i-- {
		if state.manifests[i].Targets != nil {
			combined.Targets = append([]domain.TargetConfig(nil), state.manifests[i].Targets...)
			break
		}
	}
	if combined.Version == 0 {
		combined.Version = 1
	}
	return combined, nil
}

func mergeSkills(manifests []*domain.Manifest) []domain.SkillEntry {
	merged := []domain.SkillEntry{}
	positions := map[string]int{}
	for _, manifest := range manifests {
		for _, entry := range manifest.Skills {
			if position, ok := positions[entry.Name]; ok {
				merged[position] = entry
				continue
			}
			positions[entry.Name] = len(merged)
			merged = append(merged, entry)
		}
	}
	return merged
}

func mergeConfigs(manifests []*domain.Manifest) []domain.ConfigEntry {
	merged := []domain.ConfigEntry{}
	positions := map[string]int{}
	for _, manifest := range manifests {
		for _, entry := range manifest.Configs {
			if position, ok := positions[entry.Name]; ok {
				merged[position] = entry
				continue
			}
			positions[entry.Name] = len(merged)
			merged = append(merged, entry)
		}
	}
	return merged
}

// ReadSkill reads the last declaration of a named skill, allowing repository
// context to override a shared skill with the same name.
func (s *Source) ReadSkill(ctx context.Context, name string) (*domain.Skill, error) {
	state, err := s.readSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	for i := len(state.manifests) - 1; i >= 0; i-- {
		for j := len(state.manifests[i].Skills) - 1; j >= 0; j-- {
			if state.manifests[i].Skills[j].Name == name {
				return s.layers[i].ReadSkill(ctx, name)
			}
		}
	}
	return nil, fmt.Errorf("skill not found in layered source: %s", name)
}

// ListSkills returns the composed skill declarations.
func (s *Source) ListSkills(ctx context.Context) ([]domain.SkillInfo, error) {
	manifest, err := s.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.SkillInfo, 0, len(manifest.Skills))
	for _, entry := range manifest.Skills {
		result = append(result, domain.SkillInfo(entry))
	}
	return result, nil
}

// ReadConfig reads the last declaration of a named config, allowing repository
// context to override a shared config with the same name.
func (s *Source) ReadConfig(ctx context.Context, name string) (*domain.ConfigFile, error) {
	state, err := s.readSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	for i := len(state.manifests) - 1; i >= 0; i-- {
		for j := len(state.manifests[i].Configs) - 1; j >= 0; j-- {
			if state.manifests[i].Configs[j].Name == name {
				return s.layers[i].ReadConfig(ctx, name)
			}
		}
	}
	return nil, fmt.Errorf("config not found in layered source: %s", name)
}

// ListConfigs returns the composed config declarations.
func (s *Source) ListConfigs(ctx context.Context) ([]domain.ConfigInfo, error) {
	manifest, err := s.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ConfigInfo, 0, len(manifest.Configs))
	for _, entry := range manifest.Configs {
		result = append(result, domain.ConfigInfo(entry))
	}
	return result, nil
}
