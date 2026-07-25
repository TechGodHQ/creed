package ports

// Watcher is the filesystem-watching port used by the watch use case.
//
// Implementations must deliver events for the watched roots on Events
// and surface watcher-level failures on Errors. Add is called once per
// root path to register; Close releases all underlying resources.
type Watcher interface {
	// Add registers a recursive watch rooted at path.
	Add(path string) error
	// Events returns the channel of filesystem events.
	Events() <-chan WatchEvent
	// Errors returns the channel of watcher-level errors.
	Errors() <-chan error
	// Close releases all watch resources.
	Close() error
}

// WatchEvent is a normalized filesystem change notification.
type WatchEvent struct {
	// Path is the absolute or working-directory-relative path that changed.
	Path string
	// Op is a bitfield-style description of what changed.
	Op WatchOp
}

// WatchOp describes a single or combined filesystem operation.
type WatchOp uint32

// Recognized operations. Implementations may OR them together for a
// single event covering multiple operations on the same path.
const (
	WatchOpCreate WatchOp = 1 << iota
	WatchOpWrite
	WatchOpRemove
	WatchOpRename
)
