// Package usecase implements the application-layer use cases for creed.
// The SyncEngine orchestrates source-to-target synchronization by reading
// from a SourceReader, preparing files per target, and emitting via a
// TargetEmitter.
package usecase

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

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
	targetNames, err := resolveTargets(manifest, opts)
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
	result := &SyncResult{Targets: make([]TargetResult, 0, len(targetNames))}
	for _, name := range targetNames {
		// Respect context cancellation between targets.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		result.Targets = append(result.Targets, e.syncTarget(ctx, name, skills, configs, opts))
	}

	return result, nil
}

// resolveTargets determines the set of target names to sync.
func resolveTargets(manifest *domain.Manifest, opts SyncOptions) ([]string, error) {
	if opts.Target != "" {
		// A specific target was requested. It must exist in the manifest.
		for _, tc := range manifest.Targets {
			if tc.Name == opts.Target {
				return []string{opts.Target}, nil
			}
		}
		return nil, fmt.Errorf("target %q not found in manifest", opts.Target)
	}

	// No specific target: sync all enabled targets, sorted for determinism.
	names := make([]string, 0, len(manifest.Targets))
	for _, tc := range manifest.Targets {
		if tc.Enabled {
			names = append(names, tc.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// syncTarget syncs a single target and returns its result.
func (e *SyncEngine) syncTarget(
	ctx context.Context,
	name string,
	skills []domain.Skill,
	configs []domain.ConfigFile,
	opts SyncOptions,
) TargetResult {
	start := time.Now()
	tr := TargetResult{Target: name}

	// Look up the target definition from the domain registry.
	target, err := domain.LookupTarget(name)
	if err != nil {
		tr.Error = fmt.Errorf("unknown target %q: %w", name, err)
		tr.Duration = time.Since(start)
		return tr
	}

	// Prepare the files to emit for this target.
	files := prepareFiles(target, skills, configs)

	// Dry-run: report what would change without writing.
	if opts.DryRun {
		for _, f := range files {
			tr.Files = append(tr.Files, FileResult{
				Path:   f.Path,
				Status: StatusWouldWrite,
			})
		}
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

// prepareFiles builds the list of files to emit for a target based on
// its emit paths and the available skills and configs.
//
// Directory emit paths (ending in "/") receive individual skill files.
// The first file emit path receives the aggregated content of all config
// files (the "main context file" for the target). Additional file paths
// are skipped — targets with multiple file paths (e.g., aider's
// .aider.conf.yml + CONVENTIONS.md) need per-path content semantics that
// the current generic engine does not yet support.
func prepareFiles(target *domain.Target, skills []domain.Skill, configs []domain.ConfigFile) []ports.EmittedFile {
	var files []ports.EmittedFile
	filePathSeen := false

	for _, emitPath := range target.EmitPaths("") {
		if isDirPath(emitPath) {
			// Directory: emit each skill as an individual file.
			for _, skill := range skills {
				files = append(files, ports.EmittedFile{
					Path:    emitPath + skill.Name + ".md",
					Content: skill.Content,
				})
			}
		} else if !filePathSeen {
			// First file path: aggregate all config content into this file.
			filePathSeen = true
			content := aggregateConfigs(configs)
			if len(content) > 0 {
				files = append(files, ports.EmittedFile{
					Path:    emitPath,
					Content: content,
				})
			}
		}
		// Additional file paths are intentionally skipped (see doc comment).
	}

	return files
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
