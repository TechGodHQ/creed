package ports

import (
	"context"

	"github.com/techgodhq/creed/internal/domain"
)

// EmittedFile represents a single file to be written to a target output location.
type EmittedFile struct {
	// Path is the relative path where the file should be written,
	// relative to the target's output directory.
	Path string
	// Content is the raw file content to write.
	Content []byte
}

// EmitResult captures the outcome of emitting a single file.
type EmitResult struct {
	// Path is the file path that was emitted.
	Path string
	// Status is the emit status: "written", "skipped", or "error".
	Status string
	// Error holds any error that occurred, or nil on success.
	Error error
}

// Emit status constants used in EmitResult.Status.
const (
	EmitStatusWritten = "written"
	EmitStatusSkipped = "skipped"
	EmitStatusError   = "error"
)

// TargetEmitter is the port for writing synced files to a target's output location.
// Implementations MUST NOT expose filesystem or network internals to callers —
// only the domain-level abstractions defined here.
type TargetEmitter interface {
	// Emit writes the given files to the target's output directory.
	// Files whose content matches the existing file on disk are skipped.
	// Returns per-file results indicating written/skipped/error status.
	Emit(ctx context.Context, target domain.Target, files []EmittedFile) ([]EmitResult, error)

	// Clean removes all files and directories associated with the target
	// from the output location.
	Clean(ctx context.Context, target domain.Target) error
}
