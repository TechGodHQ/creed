package usecase

// SyncOptions controls the behavior of a sync operation.
type SyncOptions struct {
	// Target filters the sync to a single target by name.
	// When empty, all enabled targets in the manifest are synced.
	// When non-empty, only the named target is synced (it must exist
	// in the manifest, regardless of its enabled state).
	Target string `json:"target,omitempty"`

	// DryRun, when true, reports the candidate file set that would be
	// emitted without writing anything to disk. Files are reported with
	// status "would_write". Note: dry-run reports all candidate files,
	// not the delta — files that are already up-to-date and would be
	// skipped on a real run are still reported as "would_write" in v1.
	// True delta reporting would require a preview/diff port method.
	DryRun bool `json:"dry_run,omitempty"`

	// Force, when true, causes identical files to be overwritten
	// instead of skipped. The target's existing files are cleaned
	// before emitting so all files are rewritten fresh.
	Force bool `json:"force,omitempty"`
}
