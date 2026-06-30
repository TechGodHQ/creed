package usecase

// SyncOptions controls the behavior of a sync operation.
type SyncOptions struct {
	// Target filters the sync to a single target by name.
	// When empty, all enabled targets in the manifest are synced.
	// When non-empty, only the named target is synced (it must exist
	// in the manifest, regardless of its enabled state).
	Target string

	// DryRun, when true, reports what would change without writing
	// any files to disk. Files are reported with status "would_write".
	DryRun bool

	// Force, when true, causes identical files to be overwritten
	// instead of skipped. The target's existing files are cleaned
	// before emitting so all files are rewritten fresh.
	Force bool
}
