package localfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/techgodhq/creed/internal/domain"
	"github.com/techgodhq/creed/internal/ports"
)

// Compile-time assertions for Emitter capabilities.
var _ ports.TargetEmitter = (*Emitter)(nil)
var _ ports.OutputInventory = (*Emitter)(nil)

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
	for _, result := range results {
		if result.Status == ports.EmitStatusError {
			return results, nil
		}
	}
	if err := e.writeOwnedPaths(target.Name, files); err != nil {
		return results, fmt.Errorf("record managed output: %w", err)
	}

	return results, nil
}

// ownershipPath is deliberately kept below .creed/ so stale detection only
// considers files Creed previously emitted, never unrelated user files that
// happen to share a target output directory.
func (e *Emitter) ownershipPath(targetName string) string {
	return filepath.Join(e.baseDir, ".creed", ".outputs", targetName+".json")
}

// ownershipDirectory returns the directory used for Creed's private output
// inventory. Existing components must be directories, never symlinks, so
// ownership metadata written during sync cannot escape the project.
func (e *Emitter) ownershipDirectory(create bool) (string, error) {
	current := e.baseDir
	for _, part := range []string{".creed", ".outputs"} {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if !create {
				return current, nil
			}
			if err := os.Mkdir(current, 0755); err != nil && !os.IsExist(err) {
				return "", err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("ownership directory %q must be a non-symlink directory", current)
		}
	}
	return current, nil
}

func (e *Emitter) safeOwnershipPath(targetName string, create bool) (string, error) {
	directory, err := e.ownershipDirectory(create)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, targetName+".json"), nil
}

func (e *Emitter) writeOwnedPaths(targetName string, files []ports.EmittedFile) error {
	// Retain paths from prior successful syncs. Sync intentionally does not
	// delete removed render outputs, so replacing this list would make a stale
	// formerly-generated file invisible to `creed diff` after the next sync.
	owned, err := e.ownedPaths(domain.Target{Name: targetName})
	if err != nil {
		return err
	}
	paths := make(map[string]struct{}, len(owned)+len(files))
	for _, path := range owned {
		paths[path] = struct{}{}
	}
	for _, file := range files {
		path, err := cleanOutputPath(file.Path)
		if err != nil {
			return err
		}
		paths[path] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	data, err := json.Marshal(ordered)
	if err != nil {
		return err
	}
	path, err := e.safeOwnershipPath(targetName, true)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (e *Emitter) ownedPaths(target domain.Target) ([]string, error) {
	ownershipPath, err := e.safeOwnershipPath(target.Name, false)
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(ownershipPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("ownership manifest must not be a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	data, err := os.ReadFile(ownershipPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rawPaths []string
	if err := json.Unmarshal(data, &rawPaths); err != nil {
		return nil, fmt.Errorf("decode ownership manifest: %w", err)
	}
	paths := make([]string, 0, len(rawPaths))
	for _, path := range rawPaths {
		clean, err := cleanOutputPath(path)
		if err != nil {
			return nil, fmt.Errorf("invalid ownership path %q: %w", path, err)
		}
		paths = append(paths, clean)
	}
	return paths, nil
}

func cleanOutputPath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", fmt.Errorf("must be a non-empty relative path")
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("must not escape the project root")
	}
	return filepath.ToSlash(clean), nil
}

func (e *Emitter) safeOutputPath(relPath string) (string, error) {
	clean, err := cleanOutputPath(relPath)
	if err != nil {
		return "", err
	}
	current := e.baseDir
	for _, part := range strings.Split(filepath.FromSlash(clean), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return current, nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("output path %q traverses a symlink", relPath)
		}
	}
	return current, nil
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

// Preview reports what Emit would do without writing to disk.
func (e *Emitter) Preview(_ context.Context, _ domain.Target, files []ports.EmittedFile) ([]ports.EmitResult, error) {
	results := make([]ports.EmitResult, 0, len(files))
	for _, f := range files {
		fullPath := filepath.Join(e.baseDir, f.Path)
		existing, err := os.ReadFile(fullPath)
		if err == nil && bytes.Equal(existing, f.Content) {
			results = append(results, ports.EmitResult{Path: f.Path, Status: ports.EmitStatusSkipped})
			continue
		}
		results = append(results, ports.EmitResult{Path: f.Path, Status: ports.EmitStatusWritten})
	}
	return results, nil
}

// ExistingFiles returns current candidate outputs and files recorded by prior
// successful emits. It never recursively adopts arbitrary user files below a
// directory-style output descriptor.
func (e *Emitter) ExistingFiles(ctx context.Context, target domain.Target, candidates []ports.EmittedFile) ([]ports.ExistingFile, error) {
	files := []ports.ExistingFile{}
	paths := make(map[string]struct{})
	for _, relPath := range target.EmitPaths("") {
		if !strings.HasSuffix(relPath, "/") {
			paths[relPath] = struct{}{}
		}
	}
	for _, candidate := range candidates {
		path, err := cleanOutputPath(candidate.Path)
		if err != nil {
			return nil, err
		}
		paths[path] = struct{}{}
	}
	owned, err := e.ownedPaths(target)
	if err != nil {
		return nil, err
	}
	for _, path := range owned {
		paths[path] = struct{}{}
	}
	for relPath := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		fullPath, err := e.safeOutputPath(relPath)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(fullPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", relPath, err)
		}
		files = append(files, ports.ExistingFile{Path: filepath.ToSlash(relPath), Content: data})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
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
