package usecase

import "time"

// Status constants for FileResult.Status.
const (
	// StatusWritten indicates a file was written or updated.
	StatusWritten = "written"
	// StatusSkipped indicates a file was already up-to-date and not modified.
	StatusSkipped = "skipped"
	// StatusFailed indicates an error occurred while emitting this file.
	StatusFailed = "failed"
	// StatusWouldWrite indicates a file would be written in dry-run mode.
	StatusWouldWrite = "would_write"
)

// SyncResult is the aggregate outcome of a sync operation across all
// processed targets.
type SyncResult struct {
	// Targets contains one TargetResult per target that was synced.
	Targets []TargetResult
}

// TargetResult captures the outcome of syncing a single target.
type TargetResult struct {
	// Target is the canonical target name.
	Target string

	// FilesWritten is the number of files written or updated.
	FilesWritten int

	// FilesSkipped is the number of files that were already up-to-date.
	FilesSkipped int

	// FilesFailed is the number of files that encountered an error.
	FilesFailed int

	// Duration is the time taken to sync this target.
	Duration time.Duration

	// Files contains per-file results for detailed reporting.
	Files []FileResult

	// Error holds a target-level error (e.g., unknown target, emit failure).
	// Per-file errors are recorded in FileResult.Error instead.
	Error error
}

// FileResult captures the outcome of emitting a single file.
type FileResult struct {
	// Path is the relative file path that was emitted.
	Path string

	// Status is the emit status: written, skipped, failed, or would_write.
	Status string

	// Error holds any per-file error, or nil on success.
	Error error
}

// HasErrors returns true if any target or file encountered an error.
func (r *SyncResult) HasErrors() bool {
	for i := range r.Targets {
		if r.Targets[i].Error != nil || r.Targets[i].FilesFailed > 0 {
			return true
		}
	}
	return false
}

// TotalFilesWritten returns the sum of files written across all targets.
func (r *SyncResult) TotalFilesWritten() int {
	total := 0
	for i := range r.Targets {
		total += r.Targets[i].FilesWritten
	}
	return total
}

// TotalFilesSkipped returns the sum of files skipped across all targets.
func (r *SyncResult) TotalFilesSkipped() int {
	total := 0
	for i := range r.Targets {
		total += r.Targets[i].FilesSkipped
	}
	return total
}
