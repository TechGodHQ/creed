// Package usecase implements the application-layer use cases for creed.
// The SyncEngine orchestrates source-to-target synchronization by reading
// from a SourceReader, preparing files per target, and emitting via a
// TargetEmitter.
package usecase

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

// previewEmitter is an optional emitter capability for dry-run diff previews.
// Implementations return EmitStatusWritten for files that would be written and
// EmitStatusSkipped for files that are already identical.
type previewEmitter interface {
	Preview(ctx context.Context, target domain.Target, files []ports.EmittedFile) ([]ports.EmitResult, error)
}

// SyncEngine orchestrates synchronization from a source to one or more targets.
// It is the core use case of creed: read context from the source, resolve
// emit paths for each target, and write files through the emitter.
type SyncEngine struct {
	source  ports.SourceReader
	emitter ports.TargetEmitter
}

// NewSyncEngine creates a SyncEngine with the given source reader and emitter.
func NewSyncEngine(source ports.SourceReader, emitter ports.TargetEmitter) *SyncEngine {
	return &SyncEngine{
		source:  source,
		emitter: emitter,
	}
}

// Sync performs a synchronization from the source to all applicable targets
// as determined by the manifest and the provided options.
//
// The operation is idempotent: running Sync twice with the same source
// produces identical file content, and the second run reports all files
// as skipped (unless Force is set).
//
// Partial failures do not abort the entire operation: if one target fails,
// remaining targets are still processed and their results are reported.
func (e *SyncEngine) Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
	// Read the manifest to discover targets, skills, and configs.
	manifest, err := e.source.ReadManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	// Resolve which targets to sync based on manifest state and options.
	targetConfigs, err := resolveTargets(manifest, opts)
	if err != nil {
		return nil, err
	}

	// Read all skills and configs referenced in the manifest.
	skills, err := readAllSkills(ctx, e.source, manifest)
	if err != nil {
		return nil, fmt.Errorf("read skills: %w", err)
	}
	configs, err := readAllConfigs(ctx, e.source, manifest)
	if err != nil {
		return nil, fmt.Errorf("read configs: %w", err)
	}

	// Sync each target independently. A failure on one target does not
	// prevent the others from being processed.
	result := &SyncResult{Targets: make([]TargetResult, 0, len(targetConfigs))}
	for _, config := range targetConfigs {
		// Respect context cancellation between targets.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		result.Targets = append(result.Targets, e.syncTarget(ctx, config, skills, configs, opts))
	}

	return result, nil
}

// resolveTargets determines the set of target configs to sync.
func resolveTargets(manifest *domain.Manifest, opts SyncOptions) ([]domain.TargetConfig, error) {
	if opts.Target != "" {
		// A specific target was requested. It must exist in the manifest.
		for _, tc := range manifest.Targets {
			if tc.Name == opts.Target {
				config := normalizeTargetConfig(tc)
				if err := validateTargetConfig(config); err != nil {
					return nil, err
				}
				return []domain.TargetConfig{config}, nil
			}
		}
		return nil, fmt.Errorf("target %q not found in manifest", opts.Target)
	}

	// No specific target: sync all enabled targets, sorted for determinism.
	configs := make([]domain.TargetConfig, 0, len(manifest.Targets))
	for _, tc := range manifest.Targets {
		if tc.Enabled {
			config := normalizeTargetConfig(tc)
			if err := validateTargetConfig(config); err != nil {
				return nil, err
			}
			configs = append(configs, config)
		}
	}
	sort.Slice(configs, func(i, j int) bool { return configs[i].Name < configs[j].Name })
	return configs, nil

}

// normalizeTargetConfig applies manifest defaults to a target config.
func normalizeTargetConfig(config domain.TargetConfig) domain.TargetConfig {
	if config.OutputDir == "" {
		config.OutputDir = "."
	}
	return config
}

func validateTargetConfig(config domain.TargetConfig) error {
	if config.OutputDir == "" || config.OutputDir == "." {
		return nil
	}
	if filepath.IsAbs(config.OutputDir) {
		return fmt.Errorf("target %q output_dir must be relative: %s", config.Name, config.OutputDir)
	}
	clean := filepath.Clean(config.OutputDir)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("target %q output_dir escapes project root: %s", config.Name, config.OutputDir)
	}
	return nil
}

// syncTarget syncs a single target and returns its result.
func (e *SyncEngine) syncTarget(
	ctx context.Context,
	config domain.TargetConfig,
	skills []domain.Skill,
	configs []domain.ConfigFile,
	opts SyncOptions,
) TargetResult {
	start := time.Now()
	name := config.Name
	tr := TargetResult{Target: name}

	// Look up the target definition from the domain registry.
	target, err := domain.LookupTarget(name)
	if err != nil {
		tr.Error = fmt.Errorf("unknown target %q: %w", name, err)
		tr.Duration = time.Since(start)
		return tr
	}

	// Prepare the files to emit for this target.
	target = targetWithOutputDir(target, config.OutputDir)
	files := prepareFiles(target, skills, configs)

	// Dry-run: report what would change without writing. Emitters that can
	// inspect their destination should report skipped for files that are already
	// identical; generic emitters fall back to reporting the candidate set.
	if opts.DryRun {
		tr = e.previewTarget(ctx, tr, target, files)
		tr.Duration = time.Since(start)
		return tr
	}

	// Force mode: clean existing files first so everything rewrites.
	if opts.Force {
		if err := e.emitter.Clean(ctx, *target); err != nil {
			tr.Error = fmt.Errorf("clean target %q: %w", name, err)
			tr.Duration = time.Since(start)
			return tr
		}
	}

	// Emit files. The emitter handles per-file skip-on-identical logic.
	emitResults, err := e.emitter.Emit(ctx, *target, files)
	if err != nil {
		tr.Error = fmt.Errorf("emit to target %q: %w", name, err)
		tr.Duration = time.Since(start)
		return tr
	}

	// Map emitter results to use-case file results and aggregate counts.
	for _, er := range emitResults {
		status := er.Status
		if status == ports.EmitStatusError {
			status = StatusFailed
		}
		fr := FileResult{Path: er.Path, Status: status, Error: er.Error}
		tr.Files = append(tr.Files, fr)

		switch status {
		case StatusWritten:
			tr.FilesWritten++
		case StatusSkipped:
			tr.FilesSkipped++
		case StatusFailed:
			tr.FilesFailed++
		}
	}

	tr.Duration = time.Since(start)
	return tr
}

// previewTarget reports dry-run results without writing files.
func (e *SyncEngine) previewTarget(
	ctx context.Context,
	tr TargetResult,
	target *domain.Target,
	files []ports.EmittedFile,
) TargetResult {
	previewer, ok := e.emitter.(previewEmitter)
	if !ok {
		for _, f := range files {
			tr.Files = append(tr.Files, FileResult{Path: f.Path, Status: StatusWouldWrite})
			tr.FilesWouldWrite++
		}
		return tr
	}

	previewResults, err := previewer.Preview(ctx, *target, files)
	if err != nil {
		tr.Error = fmt.Errorf("preview target %q: %w", target.Name, err)
		return tr
	}
	for _, er := range previewResults {
		status := er.Status
		if status == ports.EmitStatusWritten {
			status = StatusWouldWrite
		} else if status == ports.EmitStatusError {
			status = StatusFailed
		}
		tr.Files = append(tr.Files, FileResult{Path: er.Path, Status: status, Error: er.Error})
		switch status {
		case StatusWouldWrite:
			tr.FilesWouldWrite++
		case StatusSkipped:
			tr.FilesSkipped++
		case StatusFailed:
			tr.FilesFailed++
		}
	}
	return tr
}

// targetWithOutputDir returns a target whose emit paths are rooted under the
// manifest-configured output directory.
func targetWithOutputDir(target *domain.Target, outputDir string) *domain.Target {
	if outputDir == "" || outputDir == "." {
		return target
	}
	copy := *target
	if target.Outputs != nil {
		copy.Outputs = func(projectName string) []domain.TargetOutput {
			outputs := target.Outputs(projectName)
			prefixed := make([]domain.TargetOutput, 0, len(outputs))
			for _, output := range outputs {
				path := prefixOutputPath(outputDir, output.Path)
				prefixed = append(prefixed, domain.TargetOutput{
					Path:   path,
					Kind:   output.Kind,
					Format: output.Format,
				})
			}
			return prefixed
		}
	}
	copy.EmitPaths = func(projectName string) []string {
		paths := target.EmitPaths(projectName)
		prefixed := make([]string, 0, len(paths))
		for _, path := range paths {
			prefixed = append(prefixed, prefixOutputPath(outputDir, path))
		}
		return prefixed
	}
	return &copy
}

func prefixOutputPath(outputDir, path string) string {
	joined := filepath.ToSlash(filepath.Join(outputDir, path))
	if isDirPath(path) && !isDirPath(joined) {
		joined += "/"
	}
	return joined
}

// prepareFiles builds the list of files to emit for a target based on
// its semantic output descriptors and the available skills and configs.
//
// Context outputs receive aggregated config content. Skill directory outputs
// receive one markdown file per skill. Config outputs receive target-specific
// generated configuration content.
func prepareFiles(target *domain.Target, skills []domain.Skill, configs []domain.ConfigFile) []ports.EmittedFile {
	var files []ports.EmittedFile

	for _, output := range targetOutputs(target) {
		switch output.Kind {
		case domain.OutputKindContext:
			content := aggregateConfigs(configs)
			if len(content) > 0 {
				files = append(files, ports.EmittedFile{
					Path:    output.Path,
					Content: content,
				})
			}
		case domain.OutputKindSkillDir:
			for _, skill := range skills {
				files = append(files, ports.EmittedFile{
					Path:    output.Path + skill.Name + ".md",
					Content: skill.Content,
				})
			}
		case domain.OutputKindConfig:
			content := renderTargetConfig(target, output, configs, skills)
			if len(content) > 0 {
				files = append(files, ports.EmittedFile{
					Path:    output.Path,
					Content: content,
				})
			}
		}
	}

	return files
}

func targetOutputs(target *domain.Target) []domain.TargetOutput {
	if target.Outputs != nil {
		return target.Outputs("")
	}

	outputs := make([]domain.TargetOutput, 0, len(target.EmitPaths("")))
	filePathSeen := false
	for _, emitPath := range target.EmitPaths("") {
		kind := domain.OutputKindSkillDir
		if !isDirPath(emitPath) {
			if filePathSeen {
				continue
			}
			filePathSeen = true
			kind = domain.OutputKindContext
		}
		outputs = append(outputs, domain.TargetOutput{Path: emitPath, Kind: kind})
	}
	return outputs
}

func renderTargetConfig(target *domain.Target, output domain.TargetOutput, _ []domain.ConfigFile, _ []domain.Skill) []byte {
	if target.Name == "aider" && output.Kind == domain.OutputKindConfig {
		return []byte("read:\n  - CONVENTIONS.md\n")
	}
	return nil
}

// isDirPath returns true if the path represents a directory (ends with "/").
func isDirPath(path string) bool {
	return strings.HasSuffix(path, "/")
}

// aggregateConfigs concatenates all config file contents, separated by
// a markdown horizontal rule for readability.
func aggregateConfigs(configs []domain.ConfigFile) []byte {
	if len(configs) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for i, c := range configs {
		if i > 0 {
			buf.WriteString("\n---\n\n")
		}
		buf.Write(c.Content)
	}
	return buf.Bytes()
}

// readAllSkills reads the full content of every skill declared in the manifest.
func readAllSkills(ctx context.Context, source ports.SourceReader, manifest *domain.Manifest) ([]domain.Skill, error) {
	skills := make([]domain.Skill, 0, len(manifest.Skills))
	for _, entry := range manifest.Skills {
		skill, err := source.ReadSkill(ctx, entry.Name)
		if err != nil {
			return nil, fmt.Errorf("skill %q: %w", entry.Name, err)
		}
		if skill == nil {
			return nil, fmt.Errorf("skill %q: source returned nil", entry.Name)
		}
		skills = append(skills, *skill)
	}
	return skills, nil
}

// readAllConfigs reads the full content of every config declared in the manifest.
func readAllConfigs(ctx context.Context, source ports.SourceReader, manifest *domain.Manifest) ([]domain.ConfigFile, error) {
	configs := make([]domain.ConfigFile, 0, len(manifest.Configs))
	for _, entry := range manifest.Configs {
		config, err := source.ReadConfig(ctx, entry.Name)
		if err != nil {
			return nil, fmt.Errorf("config %q: %w", entry.Name, err)
		}
		if config == nil {
			return nil, fmt.Errorf("config %q: source returned nil", entry.Name)
		}
		configs = append(configs, *config)
	}
	return configs, nil
}
