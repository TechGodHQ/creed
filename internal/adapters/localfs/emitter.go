package localfs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

// Compile-time assertion that Emitter implements ports.TargetEmitter.
var _ ports.TargetEmitter = (*Emitter)(nil)

// Emitter writes synced files to a local filesystem directory.
// It implements ports.TargetEmitter.
type Emitter struct {
	// baseDir is the root directory where target files are emitted.
	// All file paths in EmittedFile are resolved relative to this directory.
	baseDir string
}

// NewEmitter creates a LocalFS target emitter that writes to the given
// base directory (typically the project root).
func NewEmitter(baseDir string) *Emitter {
	return &Emitter{baseDir: baseDir}
}

// Emit writes each file to the target's output directory.
// Files are written atomically (temp file + rename). Files whose content
// matches the existing on-disk file are skipped without modification.
// Partial failures do not abort the remaining files.
func (e *Emitter) Emit(ctx context.Context, target domain.Target, files []ports.EmittedFile) ([]ports.EmitResult, error) {
	results := make([]ports.EmitResult, 0, len(files))

	for _, f := range files {
		result := e.emitFile(f)
		results = append(results, result)
	}

	return results, nil
}

// emitFile writes a single file atomically, returning the result.
func (e *Emitter) emitFile(f ports.EmittedFile) ports.EmitResult {
	fullPath := filepath.Join(e.baseDir, f.Path)

	// Check if the file already exists with identical content.
	existing, err := os.ReadFile(fullPath)
	if err == nil && bytes.Equal(existing, f.Content) {
		return ports.EmitResult{
			Path:   f.Path,
			Status: ports.EmitStatusSkipped,
		}
	}

	// Ensure the parent directory exists.
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ports.EmitResult{
			Path:   f.Path,
			Status: ports.EmitStatusError,
			Error:  fmt.Errorf("mkdir: %w", err),
		}
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmp, err := os.CreateTemp(dir, ".creed-emit-*")
	if err != nil {
		return ports.EmitResult{
			Path:   f.Path,
			Status: ports.EmitStatusError,
			Error:  fmt.Errorf("create temp file: %w", err),
		}
	}

	// Ensure the file is world-readable (0644), matching typical source files.
	// os.CreateTemp creates files with mode 0600, which would make synced
	// files unreadable by other users, CI runners, or Docker containers.
	if err := os.Chmod(tmp.Name(), 0644); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return ports.EmitResult{
			Path:   f.Path,
			Status: ports.EmitStatusError,
			Error:  fmt.Errorf("chmod temp file: %w", err),
		}
	}

	// Write content to temp file.
	if _, err := tmp.Write(f.Content); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return ports.EmitResult{
			Path:   f.Path,
			Status: ports.EmitStatusError,
			Error:  fmt.Errorf("write temp file: %w", err),
		}
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return ports.EmitResult{
			Path:   f.Path,
			Status: ports.EmitStatusError,
			Error:  fmt.Errorf("close temp file: %w", err),
		}
	}

	// Atomic rename.
	if err := os.Rename(tmp.Name(), fullPath); err != nil {
		os.Remove(tmp.Name())
		return ports.EmitResult{
			Path:   f.Path,
			Status: ports.EmitStatusError,
			Error:  fmt.Errorf("rename: %w", err),
		}
	}

	return ports.EmitResult{
		Path:   f.Path,
		Status: ports.EmitStatusWritten,
	}
}

// Clean removes all files and directories that the target would emit.
// It uses the target's EmitPaths to determine what to remove.
func (e *Emitter) Clean(ctx context.Context, target domain.Target) error {
	if target.EmitPaths == nil {
		return nil
	}
	for _, relPath := range target.EmitPaths("") {
		fullPath := filepath.Join(e.baseDir, relPath)
		// RemoveAll handles both files and directories gracefully.
		if err := os.RemoveAll(fullPath); err != nil {
			return fmt.Errorf("clean %s: %w", relPath, err)
		}
	}
	return nil
}
