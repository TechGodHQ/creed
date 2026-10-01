package service

import (
	"context"
	"fmt"

	"github.com/techgodhq/creed/internal/adapters/gitremote"
	"github.com/techgodhq/creed/internal/adapters/layered"
	"github.com/techgodhq/creed/internal/adapters/localfs"
	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

type sourceHandle struct {
	reader   ports.SourceReader
	layers   []ports.SourceReader
	cleanups []func()
}

// manifestOverrideSource supplies an in-memory manifest while retaining the
// local reader for project-owned skills and config files. Pull uses it to
// preview a candidate layered manifest before that manifest is persisted.
type manifestOverrideSource struct {
	ports.SourceReader
	manifest *domain.Manifest
}

func (s manifestOverrideSource) ReadManifest(context.Context) (*domain.Manifest, error) {
	return s.manifest, nil
}

func (h *sourceHandle) close() {
	for i := len(h.cleanups) - 1; i >= 0; i-- {
		h.cleanups[i]()
	}
}

// openSource resolves the project's configured source into the same
// SourceReader used by sync, diff, list, validate, and doctor operations.
func (s *Implementation) openSource(ctx context.Context) (*sourceHandle, error) {
	control := localfs.NewSource(s.root)
	manifest, err := control.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}
	return s.openSourceForManifest(manifest)
}

func (s *Implementation) openSourceForManifest(manifest *domain.Manifest) (*sourceHandle, error) {
	if manifest == nil {
		return nil, fmt.Errorf("source manifest is nil")
	}
	sourceType := manifest.Source.Type
	if sourceType == "" {
		sourceType = "local"
	}

	switch sourceType {
	case "local":
		local := manifestOverrideSource{SourceReader: localfs.NewSource(s.root), manifest: manifest}
		if len(manifest.Source.Layers) == 0 {
			return &sourceHandle{reader: local}, nil
		}
		return s.openLayeredSource(manifest.Source.Layers, local)
	case "layered":
		if len(manifest.Source.Layers) == 0 {
			return nil, fmt.Errorf("layered source requires at least one layer")
		}
		local := manifestOverrideSource{SourceReader: localfs.NewSource(s.root), manifest: manifest}
		return s.openLayeredSource(manifest.Source.Layers, local)
	case "git":
		if len(manifest.Source.Layers) > 0 {
			return nil, fmt.Errorf("git source cannot declare source layers; use type: layered")
		}
		remote, cleanup, err := s.openGitSource(gitremote.SourceOptions{
			RemoteURL:  manifest.Source.Remote,
			SourcePath: sourcePathOrDefault(manifest.Source.Path),
			Ref:        manifest.Source.Ref,
		})
		if err != nil {
			return nil, err
		}
		return &sourceHandle{reader: remote, layers: []ports.SourceReader{remote}, cleanups: []func(){cleanup}}, nil
	default:
		return nil, fmt.Errorf("source type %q is unsupported", manifest.Source.Type)
	}
}

func (s *Implementation) openLayeredSource(layers []domain.SourceLayer, local ports.SourceReader) (*sourceHandle, error) {
	readers := make([]ports.SourceReader, 0, len(layers)+1)
	cleanups := make([]func(), 0, len(layers))
	for i, layer := range layers {
		switch layer.Type {
		case "local":
			readers = append(readers, localfs.NewSourceWithPath(s.root, sourcePathOrDefault(layer.Path)))
		case "git":
			remote, cleanup, err := s.openGitSource(gitremote.SourceOptions{
				RemoteURL:  layer.Remote,
				SourcePath: sourcePathOrDefault(layer.Path),
				Ref:        layer.Ref,
			})
			if err != nil {
				for j := len(cleanups) - 1; j >= 0; j-- {
					cleanups[j]()
				}
				return nil, fmt.Errorf("open source layer %d: %w", i, err)
			}
			readers = append(readers, remote)
			cleanups = append(cleanups, cleanup)
		default:
			for j := len(cleanups) - 1; j >= 0; j-- {
				cleanups[j]()
			}
			return nil, fmt.Errorf("source layer %d has unsupported type %q", i, layer.Type)
		}
	}
	readers = append(readers, local)
	return &sourceHandle{reader: layered.NewSource(readers...), layers: readers, cleanups: cleanups}, nil
}

func (s *Implementation) openGitSource(options gitremote.SourceOptions) (*gitremote.Source, func(), error) {
	options.Token = s.token
	options.CacheDir = s.cacheDir
	remote := gitremote.NewSourceWithOptions(options)
	if s.cacheDir == "" {
		return remote, func() { _ = remote.Cleanup() }, nil
	}
	return remote, func() {}, nil
}

func sourcePathOrDefault(path string) string {
	if path == "" {
		return ".creed"
	}
	return path
}
