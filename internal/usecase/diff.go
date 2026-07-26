package usecase

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

// DiffResult contains stable unified diffs for selected target outputs.
type DiffResult struct {
	Targets []TargetDiff
}

// TargetDiff is the diff for one target.
type TargetDiff struct {
	Target string
	Files  []FileDiff
}

// FileDiff is a unified diff for one output path.
type FileDiff struct {
	Path string
	Diff string
}

// HasDifferences reports whether any selected target output differs.
func (r *DiffResult) HasDifferences() bool {
	for _, target := range r.Targets {
		if len(target.Files) > 0 {
			return true
		}
	}
	return false
}

// UnifiedDiff joins per-file diffs in stable target and path order.
func (r *DiffResult) UnifiedDiff() string {
	var b strings.Builder
	for _, target := range r.Targets {
		for _, file := range target.Files {
			b.WriteString(file.Diff)
		}
	}
	return b.String()
}

// Diff renders the same candidate files as Sync and compares them with files
// currently owned by each target, including stale files no longer rendered.
func (e *SyncEngine) Diff(ctx context.Context, opts DiffOptions) (*DiffResult, error) {
	inventory, ok := e.emitter.(ports.OutputInventory)
	if !ok {
		return nil, fmt.Errorf("emitter does not support output inventory")
	}
	manifest, err := e.source.ReadManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	targets, err := resolveTargets(manifest, SyncOptions{Target: opts.Target})
	if err != nil {
		return nil, err
	}
	skills, err := readAllSkills(ctx, e.source, manifest)
	if err != nil {
		return nil, fmt.Errorf("read skills: %w", err)
	}
	configs, err := readAllConfigs(ctx, e.source, manifest)
	if err != nil {
		return nil, fmt.Errorf("read configs: %w", err)
	}
	result := &DiffResult{Targets: make([]TargetDiff, 0, len(targets))}
	for _, config := range targets {
		target, err := domain.LookupTarget(config.Name)
		if err != nil {
			return nil, err
		}
		target = targetWithOutputDir(target, config.OutputDir)
		desired, err := prepareFiles(target, skills, configs)
		if err != nil {
			return nil, fmt.Errorf("render target %q: %w", config.Name, err)
		}
		existing, err := inventory.ExistingFiles(ctx, *target, desired)
		if err != nil {
			return nil, fmt.Errorf("inventory target %q: %w", config.Name, err)
		}
		want, have := map[string][]byte{}, map[string][]byte{}
		for _, file := range desired {
			want[file.Path] = file.Content
		}
		for _, file := range existing {
			have[file.Path] = file.Content
		}
		paths := map[string]bool{}
		for path := range want {
			paths[path] = true
		}
		for path := range have {
			paths[path] = true
		}
		ordered := make([]string, 0, len(paths))
		for path := range paths {
			ordered = append(ordered, path)
		}
		sort.Strings(ordered)
		targetDiff := TargetDiff{Target: config.Name}
		for _, path := range ordered {
			old, oldOK := have[path]
			new, newOK := want[path]
			if oldOK && newOK && bytes.Equal(old, new) {
				continue
			}
			targetDiff.Files = append(targetDiff.Files, FileDiff{
				Path: path,
				Diff: unifiedFileDiff(path, old, new, oldOK, newOK),
			})
		}
		result.Targets = append(result.Targets, targetDiff)
	}
	return result, nil
}

func unifiedFileDiff(path string, old, new []byte, hasOld, hasNew bool) string {
	oldLabel, newLabel := "a/"+path, "b/"+path
	if !hasOld {
		oldLabel = "/dev/null"
	}
	if !hasNew {
		newLabel = "/dev/null"
	}
	oldLines, newLines := splitLines(old), splitLines(new)
	if len(oldLines) == 0 && len(newLines) == 0 && hasOld != hasNew {
		return emptyFileDiff(path, hasOld)
	}
	operations := lineOperations(oldLines, newLines)
	oldStart, newStart := 0, 0
	if len(oldLines) > 0 {
		oldStart = 1
	}
	if len(newLines) > 0 {
		newStart = 1
	}
	var b strings.Builder
	// Emit one complete-file hunk. It is deliberately context-rich rather than
	// attempting to coalesce sparse LCS edits: a unified hunk must include every
	// unchanged line between edits in its declared range to remain applicable.
	fmt.Fprintf(&b, "--- %s\n+++ %s\n@@ -%d,%d +%d,%d @@\n", oldLabel, newLabel, oldStart, len(oldLines), newStart, len(newLines))
	for _, operation := range operations {
		b.WriteByte(operation.kind)
		b.WriteString(operation.line.text)
		b.WriteByte('\n')
		if !operation.line.hasNewline {
			b.WriteString("\\ No newline at end of file\n")
		}
	}
	return b.String()
}

func emptyFileDiff(path string, deleted bool) string {
	if deleted {
		return fmt.Sprintf("diff --git a/%[1]s b/%[1]s\ndeleted file mode 100644\nindex e69de29..0000000\n--- a/%[1]s\n+++ /dev/null\n", path)
	}
	return fmt.Sprintf("diff --git a/%[1]s b/%[1]s\nnew file mode 100644\nindex 0000000..e69de29\n--- /dev/null\n+++ b/%[1]s\n", path)
}

type diffLine struct {
	text       string
	hasNewline bool
}

type diffOperation struct {
	kind             byte
	line             diffLine
	oldLine, newLine int
}

// lineOperations computes a smallest insert/delete edit script using LCS.
func lineOperations(oldLines, newLines []diffLine) []diffOperation {
	dp := make([][]int, len(oldLines)+1)
	for i := range dp {
		dp[i] = make([]int, len(newLines)+1)
	}
	for i := len(oldLines) - 1; i >= 0; i-- {
		for j := len(newLines) - 1; j >= 0; j-- {
			if oldLines[i] == newLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	operations := []diffOperation{}
	for i, j := 0, 0; i < len(oldLines) || j < len(newLines); {
		switch {
		case i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j]:
			operations = append(operations, diffOperation{kind: ' ', line: oldLines[i], oldLine: i + 1, newLine: j + 1})
			i, j = i+1, j+1
		case j < len(newLines) && (i == len(oldLines) || dp[i][j+1] > dp[i+1][j]):
			operations = append(operations, diffOperation{kind: '+', line: newLines[j], oldLine: i, newLine: j + 1})
			j++
		default:
			operations = append(operations, diffOperation{kind: '-', line: oldLines[i], oldLine: i + 1, newLine: j})
			i++
		}
	}
	return operations
}

func splitLines(content []byte) []diffLine {
	if len(content) == 0 {
		return nil
	}
	text := string(content)
	parts := strings.Split(text, "\n")
	endsWithNewline := strings.HasSuffix(text, "\n")
	if endsWithNewline {
		parts = parts[:len(parts)-1]
	}
	lines := make([]diffLine, 0, len(parts))
	for i, part := range parts {
		lines = append(lines, diffLine{text: part, hasNewline: i < len(parts)-1 || endsWithNewline})
	}
	return lines
}
