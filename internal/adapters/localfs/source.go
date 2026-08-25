// Package localfs implements the LocalFS source adapter for reading creed
// data from a local filesystem directory.
package localfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
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

type sourceLayerYAML struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`
	Path   string `yaml:"path"`
	Remote string `yaml:"remote"`
	Ref    string `yaml:"ref"`
}

type sourceConfigYAML struct {
	Type     string            `yaml:"type"`
	Path     string            `yaml:"path"`
	Remote   string            `yaml:"remote"`
	Ref      string            `yaml:"ref"`
	Layers   []sourceLayerYAML `yaml:"layers"`
	Overlays []sourceLayerYAML `yaml:"overlays"`
}

type targetConfigYAML struct {
	Name      string `yaml:"name"`
	Enabled   bool   `yaml:"enabled"`
	OutputDir string `yaml:"output_dir"`
}

// Source reads creed data from a local filesystem directory.
// It implements ports.SourceReader.
type Source struct {
	// rootDir is the project/clone root used to contain the source directory.
	rootDir string
	// creedDir is the absolute path to the source directory.
	creedDir string
	// pathErr records an invalid configured source path for deferred reporting
	// from the SourceReader methods (constructors intentionally do not return
	// errors to preserve the adapter's existing API).
	pathErr error
}

// Compile-time assertion that Source implements ports.SourceReader.
var _ ports.SourceReader = (*Source)(nil)

// NewSource creates a LocalFS source reader for the given project root.
// The creed directory is resolved as root/.creed.
func NewSource(root string) *Source {
	return NewSourceWithPath(root, ".creed")
}

// NewSourceWithPath creates a LocalFS source reader rooted at a directory
// relative to root. Invalid paths are reported by the first read operation.
func NewSourceWithPath(root, sourcePath string) *Source {
	if strings.TrimSpace(sourcePath) == "" {
		sourcePath = ".creed"
	}
	clean, err := safeRelativePath(sourcePath)
	if err != nil {
		return &Source{pathErr: fmt.Errorf("invalid source path %q: %w", sourcePath, err)}
	}
	return &Source{rootDir: root, creedDir: filepath.Join(root, clean)}
}

// newSourceWithDir creates a LocalFS source reader with an explicit source directory.
// Used internally and by GitRemote to read from a cloned repo.
//
//nolint:unused // consumed by gitremote adapter in stacked PR #2
func newSourceWithDir(creedDir string) *Source {
	return &Source{rootDir: creedDir, creedDir: creedDir}
}

// ReadManifest reads and parses the manifest.yaml from the .creed/ directory.
func (s *Source) ReadManifest(ctx context.Context) (*domain.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ready(); err != nil {
		return nil, err
	}
	manifestPath, err := s.sourceFilePath("manifest.yaml")
	if err != nil {
		return nil, fmt.Errorf("invalid manifest path: %w", err)
	}

	data, err := readContainedFile(s.rootDir, manifestPath)
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

	source := domain.SourceConfig{
		Type:   mf.Source.Type,
		Path:   mf.Source.Path,
		Remote: mf.Source.Remote,
		Ref:    mf.Source.Ref,
	}
	for _, layer := range append(append([]sourceLayerYAML{}, mf.Source.Layers...), mf.Source.Overlays...) {
		source.Layers = append(source.Layers, domain.SourceLayer{
			Name:   layer.Name,
			Type:   layer.Type,
			Path:   layer.Path,
			Remote: layer.Remote,
			Ref:    layer.Ref,
		})
	}
	m := &domain.Manifest{
		Version: mf.Version,
		Source:  source,
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

// ReadManifestBytes reads the manifest through the same contained-file
// boundary as normal source reads. It is intended for strict schema validators.
func (s *Source) ReadManifestBytes(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	manifestPath, err := s.sourceFilePath("manifest.yaml")
	if err != nil {
		return nil, err
	}
	return readContainedFile(s.rootDir, manifestPath)
}

// ReadSkill reads a skill's full content by name.
func (s *Source) ReadSkill(ctx context.Context, name string) (*domain.Skill, error) {
	manifest, err := s.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}

	for i := len(manifest.Skills) - 1; i >= 0; i-- {
		entry := manifest.Skills[i]
		if entry.Name == name {
			skillPath, err := s.sourceFilePath(entry.Path)
			if err != nil {
				return nil, fmt.Errorf("invalid skill path %s: %w", entry.Path, err)
			}
			content, err := readContainedFile(s.rootDir, skillPath)
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
		skills = append(skills, domain.SkillInfo(entry))
	}
	return skills, nil
}

// ReadConfig reads a config file's full content by name.
func (s *Source) ReadConfig(ctx context.Context, name string) (*domain.ConfigFile, error) {
	manifest, err := s.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}

	for i := len(manifest.Configs) - 1; i >= 0; i-- {
		entry := manifest.Configs[i]
		if entry.Name == name {
			configPath, err := s.sourceFilePath(entry.Path)
			if err != nil {
				return nil, fmt.Errorf("invalid config path %s: %w", entry.Path, err)
			}
			content, err := readContainedFile(s.rootDir, configPath)
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
		configs = append(configs, domain.ConfigInfo(entry))
	}
	return configs, nil
}

func readContainedFile(rootDir, path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("source path must be a regular file")
	}
	if runtime.GOOS == "linux" {
		fdPath := filepath.Join("/proc/self/fd", strconv.FormatUint(uint64(file.Fd()), 10))
		resolvedFD, err := os.Readlink(fdPath)
		if err != nil {
			return nil, fmt.Errorf("resolve opened source file: %w", err)
		}
		if strings.HasSuffix(resolvedFD, " (deleted)") {
			return nil, fmt.Errorf("opened source file was deleted")
		}
		resolvedRoot, err := filepath.EvalSymlinks(rootDir)
		if err != nil {
			return nil, err
		}
		resolvedRoot, err = filepath.Abs(resolvedRoot)
		if err != nil {
			return nil, err
		}
		resolvedPath, err := filepath.EvalSymlinks(resolvedFD)
		if err != nil {
			return nil, err
		}
		requestedPath, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, err
		}
		resolvedPath, err = filepath.Abs(resolvedPath)
		if err != nil {
			return nil, err
		}
		requestedPath, err = filepath.Abs(requestedPath)
		if err != nil {
			return nil, err
		}
		if filepath.Clean(resolvedPath) != filepath.Clean(requestedPath) {
			return nil, fmt.Errorf("source file changed through a symlink during open")
		}
		relative, err := filepath.Rel(resolvedRoot, resolvedPath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("opened source file escaped source root")
		}
	}
	return io.ReadAll(file)
}

func (s *Source) ready() error {
	if s.pathErr != nil {
		return s.pathErr
	}
	if s.creedDir == "" {
		return fmt.Errorf("source directory is not configured")
	}
	return nil
}

func (s *Source) sourceFilePath(sourcePath string) (string, error) {
	if err := s.ready(); err != nil {
		return "", err
	}
	clean, err := safeRelativePath(sourcePath)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.creedDir, clean)
	rootInfo, err := os.Lstat(s.rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", fmt.Errorf("source root must be a non-symlink directory")
	}

	// Reject symlink components between the project/clone root and the
	// configured source directory, not just symlinks in the declared file path.
	relativeSource, err := filepath.Rel(s.rootDir, s.creedDir)
	if err != nil || relativeSource == ".." || strings.HasPrefix(relativeSource, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source directory must remain inside source root")
	}
	current := s.rootDir
	if relativeSource != "." {
		for _, part := range strings.Split(relativeSource, string(filepath.Separator)) {
			current = filepath.Join(current, part)
			info, statErr := os.Lstat(current)
			if os.IsNotExist(statErr) {
				return path, nil
			}
			if statErr != nil {
				return "", statErr
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("source directory %q traverses a symlink", s.creedDir)
			}
		}
	}

	sourceInfo, err := os.Lstat(s.creedDir)
	if os.IsNotExist(err) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
		return "", fmt.Errorf("source directory must be a non-symlink directory")
	}
	current = s.creedDir
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			return path, nil
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("path %q traverses a symlink", sourcePath)
		}
	}

	resolvedRoot, err := filepath.EvalSymlinks(s.rootDir)
	if err != nil {
		return "", err
	}
	resolvedSourceRoot, err := filepath.EvalSymlinks(s.creedDir)
	if err != nil {
		return "", err
	}
	if relative, relErr := filepath.Rel(resolvedRoot, resolvedSourceRoot); relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("source directory must remain inside source root")
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	if relative, relErr := filepath.Rel(resolvedSourceRoot, resolvedPath); relErr != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must remain inside source directory")
	}
	return path, nil
}

func safeRelativePath(sourcePath string) (string, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", fmt.Errorf("path is required")
	}
	path := filepath.FromSlash(sourcePath)
	if filepath.IsAbs(path) || filepath.VolumeName(path) != "" {
		return "", fmt.Errorf("path must be relative")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must remain inside source directory")
	}
	return clean, nil
}
