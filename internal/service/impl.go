package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/techgodhq/creed/internal/adapters/gitremote"
	"github.com/techgodhq/creed/internal/adapters/localfs"
	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/usecase"
)

const (
	defaultSourceDir = ".creed"
	manifestName     = "manifest.yaml"
)

// ErrUnsupportedOperation is returned for service methods whose contract is
// defined but whose persistence semantics are intentionally not implemented.
var ErrUnsupportedOperation = errors.New("unsupported operation")

// Implementation is the default local-project implementation of Service.
type Implementation struct {
	root     string
	token    string
	cacheDir string
}

var _ Service = (*Implementation)(nil)

// Option configures a Service implementation.
type Option func(*Implementation)

// WithGitToken configures an optional token used when reading git remotes.
func WithGitToken(token string) Option {
	return func(s *Implementation) { s.token = token }
}

// WithCacheDir configures the git remote cache directory used by Pull.
func WithCacheDir(cacheDir string) Option {
	return func(s *Implementation) { s.cacheDir = cacheDir }
}

// New creates a Service rooted at the given project directory.
func New(root string, opts ...Option) *Implementation {
	impl := &Implementation{root: root}
	for _, opt := range opts {
		opt(impl)
	}
	return impl
}

// Init creates a .creed directory and manifest.yaml if they do not already
// exist. Existing manifests are left intact.
func (s *Implementation) Init(ctx context.Context, projectName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.creedDir(), 0755); err != nil {
		return fmt.Errorf("create creed dir: %w", err)
	}
	if _, err := os.Stat(s.manifestPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat manifest: %w", err)
	}

	manifest := domain.NewManifest()
	manifest.Targets = defaultTargets()
	return s.writeManifest(manifest)
}

// Sync syncs local Creed context from .creed/ to configured targets.
func (s *Implementation) Sync(ctx context.Context, opts usecase.SyncOptions) (*usecase.SyncResult, error) {
	engine := usecase.NewSyncEngine(localfs.NewSource(s.root), localfs.NewEmitter(s.root))
	return engine.Sync(ctx, opts)
}

// AddSkill registers a skill path in the manifest.
func (s *Implementation) AddSkill(ctx context.Context, name, sourcePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("skill name is required")
	}
	if sourcePath == "" {
		sourcePath = filepath.ToSlash(filepath.Join("skills", name+".md"))
	}
	manifest, err := s.readManifest()
	if err != nil {
		return err
	}
	for i := range manifest.Skills {
		if manifest.Skills[i].Name == name {
			manifest.Skills[i].Path = sourcePath
			return s.writeManifest(manifest)
		}
	}
	manifest.Skills = append(manifest.Skills, domain.SkillEntry{Name: name, Path: sourcePath})
	sort.Slice(manifest.Skills, func(i, j int) bool { return manifest.Skills[i].Name < manifest.Skills[j].Name })
	return s.writeManifest(manifest)
}

// RemoveSkill removes a skill registration from the manifest.
func (s *Implementation) RemoveSkill(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	manifest, err := s.readManifest()
	if err != nil {
		return err
	}
	for i := range manifest.Skills {
		if manifest.Skills[i].Name == name {
			manifest.Skills = append(manifest.Skills[:i], manifest.Skills[i+1:]...)
			return s.writeManifest(manifest)
		}
	}
	return fmt.Errorf("skill not found: %s", name)
}

// ListSkills lists all manifest-registered skills.
func (s *Implementation) ListSkills(ctx context.Context) ([]domain.SkillInfo, error) {
	return localfs.NewSource(s.root).ListSkills(ctx)
}

// ListTargets lists all known targets and annotates them with manifest state.
func (s *Implementation) ListTargets(ctx context.Context) ([]domain.TargetInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	manifest, err := s.readManifest()
	if err != nil {
		return nil, err
	}
	configured := make(map[string]domain.TargetConfig, len(manifest.Targets))
	for _, tc := range manifest.Targets {
		configured[tc.Name] = tc
	}

	targetNames := domain.AllTargetNames()
	infos := make([]domain.TargetInfo, 0, len(targetNames))
	for _, name := range targetNames {
		target, err := domain.LookupTarget(name)
		if err != nil {
			return nil, err
		}
		cfg := configured[target.Name]
		infos = append(infos, domain.TargetInfo{
			Name:        target.Name,
			DisplayName: target.DisplayName,
			Enabled:     cfg.Enabled,
			OutputDir:   cfg.OutputDir,
			EmitPaths:   target.EmitPaths(""),
		})
	}
	return infos, nil
}

// EnableTarget enables a target in the manifest.
func (s *Implementation) EnableTarget(ctx context.Context, name string) error {
	return s.setTargetEnabled(ctx, name, true)
}

// DisableTarget disables a target in the manifest.
func (s *Implementation) DisableTarget(ctx context.Context, name string) error {
	return s.setTargetEnabled(ctx, name, false)
}

// Pull reads Creed context from a git remote and syncs it into this service's
// root using the same SyncEngine path as local sync.
func (s *Implementation) Pull(ctx context.Context, remoteURL string) error {
	if remoteURL == "" {
		manifest, err := s.readManifest()
		if err != nil {
			return err
		}
		remoteURL = manifest.Source.Remote
	}
	if remoteURL == "" {
		return fmt.Errorf("remote URL is required")
	}
	source := gitremote.NewSource(remoteURL, s.token)
	if s.cacheDir != "" {
		source = gitremote.NewSourceWithCache(remoteURL, s.token, s.cacheDir)
	} else {
		defer source.Cleanup()
	}
	engine := usecase.NewSyncEngine(source, localfs.NewEmitter(s.root))
	result, err := engine.Sync(ctx, usecase.SyncOptions{})
	if err != nil {
		return err
	}
	if result.HasErrors() {
		return fmt.Errorf("pull sync completed with target errors")
	}
	return nil
}

// Push publishes local .creed source changes to a git remote using the system
// git executable. It is intentionally isolated here until a writable git port
// exists; callers still interact through the stable Service contract.
func (s *Implementation) Push(ctx context.Context, remoteURL string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if remoteURL == "" {
		manifest, err := s.readManifest()
		if err != nil {
			return err
		}
		remoteURL = manifest.Source.Remote
	}
	if remoteURL == "" {
		return fmt.Errorf("remote URL is required")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git executable required for push: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "creed-push-*")
	if err != nil {
		return fmt.Errorf("create push workspace: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := os.RemoveAll(tmpDir); err != nil {
		return fmt.Errorf("clear push workspace: %w", err)
	}
	if err := git(ctx, "", "clone", remoteURL, tmpDir); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(tmpDir, ".creed")); err != nil {
		return fmt.Errorf("remove existing creed source: %w", err)
	}
	if err := copyDir(s.creedDir(), filepath.Join(tmpDir, ".creed")); err != nil {
		return err
	}
	if err := git(ctx, tmpDir, "add", ".creed"); err != nil {
		return err
	}
	if err := git(ctx, tmpDir, "diff", "--cached", "--quiet"); err == nil {
		return nil
	}
	if err := git(ctx, tmpDir, "-c", "user.name=Creed", "-c", "user.email=creed@techgodhq.dev", "commit", "-m", "sync creed source"); err != nil {
		return err
	}
	branch, err := gitOutput(ctx, tmpDir, "branch", "--show-current")
	if err != nil {
		return err
	}
	if branch == "" {
		branch = "main"
	}
	if err := git(ctx, tmpDir, "push", "origin", "HEAD:"+branch); err != nil {
		return err
	}
	return nil
}

func git(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read source dir: %w", err)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("create destination dir: %w", err)
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if entry.Name() == ".git" {
				continue
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", srcPath, err)
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, info.Mode()); err != nil {
			return fmt.Errorf("write %s: %w", dstPath, err)
		}
	}
	return nil
}

func (s *Implementation) setTargetEnabled(ctx context.Context, name string, enabled bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := domain.LookupTarget(name); err != nil {
		return err
	}
	manifest, err := s.readManifest()
	if err != nil {
		return err
	}
	for i := range manifest.Targets {
		if manifest.Targets[i].Name == name {
			manifest.Targets[i].Enabled = enabled
			if manifest.Targets[i].OutputDir == "" {
				manifest.Targets[i].OutputDir = "."
			}
			return s.writeManifest(manifest)
		}
	}
	manifest.Targets = append(manifest.Targets, domain.TargetConfig{Name: name, Enabled: enabled, OutputDir: "."})
	sort.Slice(manifest.Targets, func(i, j int) bool { return manifest.Targets[i].Name < manifest.Targets[j].Name })
	return s.writeManifest(manifest)
}

func (s *Implementation) creedDir() string { return filepath.Join(s.root, defaultSourceDir) }

func (s *Implementation) manifestPath() string { return filepath.Join(s.creedDir(), manifestName) }

func (s *Implementation) readManifest() (*domain.Manifest, error) {
	manifest, err := localfs.NewSource(s.root).ReadManifest(context.Background())
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func (s *Implementation) writeManifest(manifest *domain.Manifest) error {
	if manifest.Version == 0 {
		manifest.Version = 1
	}
	if manifest.Source.Type == "" {
		manifest.Source.Type = "local"
	}
	if manifest.Source.Path == "" {
		manifest.Source.Path = defaultSourceDir
	}
	if err := os.MkdirAll(s.creedDir(), 0755); err != nil {
		return fmt.Errorf("create creed dir: %w", err)
	}
	data, err := yaml.Marshal(toManifestYAML(manifest))
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	return os.WriteFile(s.manifestPath(), data, 0644)
}

func defaultTargets() []domain.TargetConfig {
	names := domain.AllTargetNames()
	configs := make([]domain.TargetConfig, 0, len(names))
	for _, name := range names {
		configs = append(configs, domain.TargetConfig{Name: name, Enabled: false, OutputDir: "."})
	}
	return configs
}

type manifestYAML struct {
	Version int                  `yaml:"version"`
	Source  sourceConfigYAML     `yaml:"source"`
	Targets []targetConfigYAML   `yaml:"targets"`
	Skills  []domain.SkillEntry  `yaml:"skills,omitempty"`
	Configs []domain.ConfigEntry `yaml:"config,omitempty"`
}

type sourceConfigYAML struct {
	Type   string `yaml:"type"`
	Path   string `yaml:"path"`
	Remote string `yaml:"remote,omitempty"`
}

type targetConfigYAML struct {
	Name      string `yaml:"name"`
	Enabled   bool   `yaml:"enabled"`
	OutputDir string `yaml:"output_dir"`
}

func toManifestYAML(manifest *domain.Manifest) manifestYAML {
	mf := manifestYAML{
		Version: manifest.Version,
		Source: sourceConfigYAML{
			Type:   manifest.Source.Type,
			Path:   manifest.Source.Path,
			Remote: manifest.Source.Remote,
		},
		Skills:  manifest.Skills,
		Configs: manifest.Configs,
	}
	for _, tc := range manifest.Targets {
		mf.Targets = append(mf.Targets, targetConfigYAML{
			Name:      tc.Name,
			Enabled:   tc.Enabled,
			OutputDir: tc.OutputDir,
		})
	}
	return mf
}
