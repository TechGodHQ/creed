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

var defaultScaffoldFiles = []scaffoldFile{
	{
		Path: "config/project.md",
		Content: `# Project Context

Describe what this project does, its product shape, and the decisions future agents must preserve.
`,
	},
	{
		Path: "config/development.md",
		Content: `# Development Instructions

Document build, test, lint, and release commands here.
`,
	},
	{
		Path: "skills/review.md",
		Content: `---
name: review
description: Guidelines for agents reviewing changes in this project.
---

# Review Guidelines

Describe how agents should review changes in this project.
`,
	},
}

type scaffoldFile struct {
	Path    string
	Content string
}

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

// Init creates a .creed directory, starter context files, and manifest.yaml if
// they do not already exist. Existing scaffold files and manifests are left intact.
func (s *Implementation) Init(ctx context.Context, projectName string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(s.creedDir(), 0755); err != nil {
		return fmt.Errorf("create creed dir: %w", err)
	}
	if err := s.writeMissingScaffoldFiles(); err != nil {
		return err
	}
	if _, err := os.Stat(s.manifestPath()); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat manifest: %w", err)
	}

	manifest := domain.NewManifest()
	manifest.Targets = defaultTargets()
	manifest.Configs = []domain.ConfigEntry{
		{Name: "project", Path: "config/project.md"},
		{Name: "development", Path: "config/development.md"},
	}
	manifest.Skills = []domain.SkillEntry{
		{Name: "review", Path: "skills/review.md"},
	}
	return s.writeManifest(manifest)
}

// Sync syncs the resolved Creed source context to configured targets.
func (s *Implementation) Sync(ctx context.Context, opts usecase.SyncOptions) (*usecase.SyncResult, error) {
	source, err := s.openSource(ctx)
	if err != nil {
		return nil, err
	}
	defer source.close()
	engine := usecase.NewSyncEngine(source.reader, localfs.NewEmitter(s.root))
	return engine.Sync(ctx, opts)
}

// Diff compares rendered resolved Creed context with its target outputs.
func (s *Implementation) Diff(ctx context.Context, opts usecase.DiffOptions) (*usecase.DiffResult, error) {
	source, err := s.openSource(ctx)
	if err != nil {
		return nil, err
	}
	defer source.close()
	engine := usecase.NewSyncEngine(source.reader, localfs.NewEmitter(s.root))
	return engine.Diff(ctx, opts)
}

// Check is the non-mutating CI-oriented alias for Diff.
func (s *Implementation) Check(ctx context.Context, opts usecase.DiffOptions) (*usecase.DiffResult, error) {
	return s.Diff(ctx, opts)
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

// ListSkills lists all skills in the resolved source, including shared layers.
// Outside a Creed project there are no registered skills, so global discovery
// succeeds with an empty result instead of requiring a manifest.
func (s *Implementation) ListSkills(ctx context.Context) ([]domain.SkillInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := os.Stat(s.manifestPath()); err != nil {
		if os.IsNotExist(err) {
			return []domain.SkillInfo{}, nil
		}
		return nil, err
	}
	source, err := s.openSource(ctx)
	if err != nil {
		return nil, err
	}
	defer source.close()
	return source.reader.ListSkills(ctx)
}

// AddConfig registers a configuration file path in the manifest.
func (s *Implementation) AddConfig(ctx context.Context, name, sourcePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("config name is required")
	}
	if sourcePath == "" {
		sourcePath = filepath.ToSlash(filepath.Join("config", name+".md"))
	}
	manifest, err := s.readManifest()
	if err != nil {
		return err
	}
	for i := range manifest.Configs {
		if manifest.Configs[i].Name == name {
			manifest.Configs[i].Path = sourcePath
			return s.writeManifest(manifest)
		}
	}
	manifest.Configs = append(manifest.Configs, domain.ConfigEntry{Name: name, Path: sourcePath})
	sort.Slice(manifest.Configs, func(i, j int) bool { return manifest.Configs[i].Name < manifest.Configs[j].Name })
	return s.writeManifest(manifest)
}

// RemoveConfig removes a configuration file registration from the manifest.
func (s *Implementation) RemoveConfig(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	manifest, err := s.readManifest()
	if err != nil {
		return err
	}
	for i := range manifest.Configs {
		if manifest.Configs[i].Name == name {
			manifest.Configs = append(manifest.Configs[:i], manifest.Configs[i+1:]...)
			return s.writeManifest(manifest)
		}
	}
	return fmt.Errorf("config not found: %s", name)
}

// ListConfigs lists all configuration files in the resolved source, including shared layers.
func (s *Implementation) ListConfigs(ctx context.Context) ([]domain.ConfigInfo, error) {
	source, err := s.openSource(ctx)
	if err != nil {
		return nil, err
	}
	defer source.close()
	return source.reader.ListConfigs(ctx)
}

// ListTargets lists all known targets and annotates them with manifest state.
// The target registry is global, so it remains useful from outside a Creed
// project; absent manifests simply mean every target is unconfigured.
func (s *Implementation) ListTargets(ctx context.Context) ([]domain.TargetInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	manifest := &domain.Manifest{}
	if _, err := os.Stat(s.manifestPath()); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else if configuredManifest, err := s.readManifest(); err != nil {
		return nil, err
	} else {
		manifest = configuredManifest
	}
	if manifest.Source.Type == "git" || manifest.Source.Type == "layered" || len(manifest.Source.Layers) > 0 {
		source, sourceErr := s.openSource(ctx)
		if sourceErr != nil {
			return nil, sourceErr
		}
		defer source.close()
		resolved, resolveErr := source.reader.ReadManifest(ctx)
		if resolveErr != nil {
			return nil, resolveErr
		}
		manifest = resolved
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
		outputs := target.Outputs("")
		infos = append(infos, domain.TargetInfo{
			Name:        target.Name,
			DisplayName: target.DisplayName,
			Enabled:     cfg.Enabled,
			OutputDir:   cfg.OutputDir,
			EmitPaths:   emitPathsFromOutputs(outputs),
			Outputs:     outputs,
		})
	}
	return infos, nil
}

func emitPathsFromOutputs(outputs []domain.TargetOutput) []string {
	paths := make([]string, len(outputs))
	for i, output := range outputs {
		paths[i] = output.Path
	}
	return paths
}

// EnableTarget enables a target in the manifest.
func (s *Implementation) EnableTarget(ctx context.Context, name string) error {
	return s.setTargetEnabled(ctx, name, true)
}

// DisableTarget disables a target in the manifest.
func (s *Implementation) DisableTarget(ctx context.Context, name string) error {
	return s.setTargetEnabled(ctx, name, false)
}

// Pull records a shared git layer and syncs the composed source into this
// project's targets. It never replaces the local .creed source files.
func (s *Implementation) Pull(ctx context.Context, opts usecase.PullOptions) (*usecase.SyncResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	previous, previousErr := s.readManifest()
	candidate, changed, err := s.pullManifest(ctx, previous, previousErr, opts.RemoteURL)
	if err != nil {
		return nil, err
	}

	var localPreview *usecase.SyncResult
	if previousErr == nil {
		localSource, err := s.openSourceForManifest(previous)
		if err != nil {
			return nil, err
		}
		defer localSource.close()
		localPreview, err = usecase.NewSyncEngine(localSource.reader, localfs.NewEmitter(s.root)).Sync(ctx, usecase.SyncOptions{DryRun: true})
		if err != nil {
			return nil, err
		}
	}

	source, err := s.openSourceForManifest(candidate)
	if err != nil {
		return nil, err
	}
	defer source.close()
	engine := usecase.NewSyncEngine(source.reader, localfs.NewEmitter(s.root))
	preview, err := engine.Sync(ctx, usecase.SyncOptions{DryRun: true})
	if err != nil {
		return nil, err
	}
	if opts.DryRun {
		return preview, nil
	}
	if !opts.Force {
		if localPreview == nil {
			localPreview = &usecase.SyncResult{}
		}
		if conflicts := pullConflicts(s.root, localPreview, preview); len(conflicts) > 0 {
			return nil, fmt.Errorf("pull would overwrite locally modified emitted files; rerun with --force: %s", strings.Join(conflicts, ", "))
		}
	}
	if changed {
		if err := s.writeManifest(candidate); err != nil {
			return nil, err
		}
	}
	result, err := engine.Sync(ctx, usecase.SyncOptions{Force: opts.Force})
	if err != nil {
		return nil, err
	}
	if result.HasErrors() {
		return result, fmt.Errorf("pull sync completed with target errors")
	}
	return result, nil
}

func (s *Implementation) pullManifest(ctx context.Context, manifest *domain.Manifest, manifestErr error, remoteURL string) (*domain.Manifest, bool, error) {
	if remoteURL == "" {
		if manifestErr != nil {
			return nil, false, manifestErr
		}
		if manifest.Source.Type != "git" && manifest.Source.Type != "layered" && len(manifest.Source.Layers) == 0 {
			return nil, false, fmt.Errorf("remote URL is required for pull")
		}
		if manifest.Source.Type == "git" && strings.TrimSpace(manifest.Source.Remote) == "" {
			return nil, false, fmt.Errorf("remote URL is required for pull")
		}
		return manifest, false, nil
	}

	normalizedRemote, err := normalizePullRemoteURL(remoteURL)
	if err != nil {
		return nil, false, err
	}
	if manifestErr != nil {
		if _, statErr := os.Stat(s.manifestPath()); statErr != nil && os.IsNotExist(statErr) {
			remote, cleanup, openErr := s.openGitSource(gitremote.SourceOptions{RemoteURL: normalizedRemote})
			if openErr != nil {
				return nil, false, openErr
			}
			defer cleanup()
			remoteManifest, readErr := remote.ReadManifest(ctx)
			if readErr != nil {
				return nil, false, readErr
			}
			return &domain.Manifest{Version: 1, Source: domain.SourceConfig{Type: "layered", Path: ".creed", Layers: []domain.SourceLayer{{Name: "org", Type: "git", Path: ".creed", Remote: normalizedRemote}}}, Targets: remoteManifest.Targets}, true, nil
		}
		return nil, false, manifestErr
	}

	candidate := cloneManifest(manifest)
	candidate.Source.Type = "layered"
	candidate.Source.Path = sourcePathOrDefault(candidate.Source.Path)
	updated := false
	for i := range candidate.Source.Layers {
		if candidate.Source.Layers[i].Name == "org" || candidate.Source.Layers[i].Remote == normalizedRemote {
			layer := candidate.Source.Layers[i]
			if layer.Name == "" {
				layer.Name = "org"
			}
			layer.Type = "git"
			if layer.Path == "" {
				layer.Path = ".creed"
			}
			layer.Remote = normalizedRemote
			candidate.Source.Layers[i] = layer
			updated = true
			break
		}
	}
	if !updated {
		candidate.Source.Layers = append(candidate.Source.Layers, domain.SourceLayer{Name: "org", Type: "git", Path: ".creed", Remote: normalizedRemote})
	}
	return candidate, true, nil
}

func cloneManifest(manifest *domain.Manifest) *domain.Manifest {
	copy := *manifest
	copy.Source.Layers = append([]domain.SourceLayer(nil), manifest.Source.Layers...)
	copy.Targets = append([]domain.TargetConfig(nil), manifest.Targets...)
	copy.Skills = append([]domain.SkillEntry(nil), manifest.Skills...)
	copy.Configs = append([]domain.ConfigEntry(nil), manifest.Configs...)
	return &copy
}

func pullConflicts(root string, local, incoming *usecase.SyncResult) []string {
	localFiles := map[string]string{}
	for _, target := range local.Targets {
		for _, file := range target.Files {
			localFiles[target.Target+"\x00"+file.Path] = file.Status
		}
	}
	conflicts := []string{}
	for _, target := range incoming.Targets {
		for _, file := range target.Files {
			key := target.Target + "\x00" + file.Path
			localStatus, locallyRendered := localFiles[key]
			if file.Status != usecase.StatusWouldWrite || (locallyRendered && localStatus != usecase.StatusWouldWrite) {
				continue
			}
			if _, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file.Path))); err == nil {
				conflicts = append(conflicts, file.Path)
			}
		}
	}
	sort.Strings(conflicts)
	return conflicts
}

// Push publishes local .creed source changes to a git remote using the system
// git executable. It is intentionally isolated here until a writable git port
// exists; callers still interact through the stable Service contract.
func (s *Implementation) Push(ctx context.Context, remoteURL string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	manifest, err := s.readManifest()
	if err != nil {
		return err
	}
	if manifest.Source.Type == "layered" || len(manifest.Source.Layers) > 0 {
		return fmt.Errorf("push is not supported for layered sources; update the shared repository through a pull request")
	}
	if remoteURL == "" {
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

func (s *Implementation) writeMissingScaffoldFiles() error {
	for _, file := range defaultScaffoldFiles {
		path := filepath.Join(s.creedDir(), file.Path)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("stat scaffold file %s: %w", path, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("create scaffold dir %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(file.Content), 0644); err != nil {
			return fmt.Errorf("write scaffold file %s: %w", path, err)
		}
	}
	return nil
}

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
		configs = append(configs, domain.TargetConfig{Name: name, Enabled: defaultTargetEnabled(name), OutputDir: "."})
	}
	return configs
}

func defaultTargetEnabled(name string) bool {
	switch name {
	case "claude", "codex", "cursor":
		return true
	default:
		return false
	}
}

type manifestYAML struct {
	Version int                  `yaml:"version"`
	Source  sourceConfigYAML     `yaml:"source"`
	Targets []targetConfigYAML   `yaml:"targets"`
	Skills  []domain.SkillEntry  `yaml:"skills,omitempty"`
	Configs []domain.ConfigEntry `yaml:"config,omitempty"`
}

type sourceLayerYAML struct {
	Name   string `yaml:"name"`
	Type   string `yaml:"type"`
	Path   string `yaml:"path"`
	Remote string `yaml:"remote,omitempty"`
	Ref    string `yaml:"ref,omitempty"`
}

type sourceConfigYAML struct {
	Type   string            `yaml:"type"`
	Path   string            `yaml:"path"`
	Remote string            `yaml:"remote,omitempty"`
	Ref    string            `yaml:"ref,omitempty"`
	Layers []sourceLayerYAML `yaml:"layers,omitempty"`
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
			Ref:    manifest.Source.Ref,
		},
		Skills:  manifest.Skills,
		Configs: manifest.Configs,
	}
	for _, layer := range manifest.Source.Layers {
		mf.Source.Layers = append(mf.Source.Layers, sourceLayerYAML{
			Name:   layer.Name,
			Type:   layer.Type,
			Path:   layer.Path,
			Remote: layer.Remote,
			Ref:    layer.Ref,
		})
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

// watchRoots returns the canonical .creed/ source paths that watch
// mode should observe. We intentionally return the manifest file and
// the specific config/skills subdirectories rather than the entire
// .creed/ tree, so unrelated files (caches, scratch, editor backups)
// do not trigger syncs. Missing subdirectories are silently skipped
// rather than erroring; a missing .creed or manifest is a hard error.
func (s *Implementation) watchRoots() ([]string, error) {
	creedDir := s.creedDir()
	info, err := os.Stat(creedDir)
	if err != nil {
		return nil, fmt.Errorf("stat creed dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("creed path is not a directory: %s", creedDir)
	}
	if _, err := os.Stat(s.manifestPath()); err != nil {
		return nil, fmt.Errorf("stat manifest: %w", err)
	}

	roots := []string{s.manifestPath()}
	for _, sub := range []string{"config", "skills"} {
		p := filepath.Join(creedDir, sub)
		if _, err := os.Stat(p); err == nil {
			roots = append(roots, p)
		}
	}
	return roots, nil
}

// Watch watches the canonical .creed/ sources and runs a debounced
// sync after each stable change. The call blocks until ctx is cancelled
// (typically via Ctrl-C in the CLI) or until the underlying watcher
// reports an unrecoverable error.
func (s *Implementation) Watch(ctx context.Context, opts usecase.WatchOptions, sink usecase.WatchSink) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	roots, err := s.watchRoots()
	if err != nil {
		return err
	}
	watcher, err := localfs.NewWatcher()
	if err != nil {
		return err
	}
	defer func() {
		_ = watcher.Close()
	}()

	syncFn := func(ctx context.Context, syncOpts usecase.SyncOptions) (*usecase.SyncResult, error) {
		source, err := s.openSource(ctx)
		if err != nil {
			return nil, err
		}
		defer source.close()
		engine := usecase.NewSyncEngine(source.reader, localfs.NewEmitter(s.root))
		return engine.Sync(ctx, syncOpts)
	}
	engine := usecase.NewWatchEngine(watcher, syncFn)
	return engine.Watch(ctx, roots, opts, sink)
}
