package localfs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"

	"github.com/techgodhq/creed/internal/ports"
)

// Watcher implements ports.Watcher by recursively registering fsnotify
// watches under the configured roots. The zero value is not usable;
// construct with NewWatcher.
type Watcher struct {
	watcher *fsnotify.Watcher
	events  chan ports.WatchEvent
	errs    chan error
}

// NewWatcher constructs a new recursive fsnotify-backed watcher.
// Close must be called to release OS resources.
func NewWatcher() (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}
	w := &Watcher{
		watcher: fsw,
		events:  make(chan ports.WatchEvent, 64),
		errs:    make(chan error, 8),
	}
	go w.pump()
	return w, nil
}

// Add registers a recursive watch rooted at path. Non-existent paths
// return an error. The path and all currently-existing descendants
// are watched; directories created later are picked up lazily by
// handling Create events for directories.
func (w *Watcher) Add(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat watch root %q: %w", path, err)
	}
	if !info.IsDir() {
		// fsnotify supports file watches; a single file is fine.
		return w.watcher.Add(path)
	}
	return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %q: %w", path, err)
		}
		if !d.IsDir() {
			return nil
		}
		if err := w.watcher.Add(p); err != nil {
			return fmt.Errorf("watch %q: %w", p, err)
		}
		return nil
	})
}

// Events returns the channel of normalized filesystem events.
func (w *Watcher) Events() <-chan ports.WatchEvent { return w.events }

// Errors returns the channel of watcher-level errors.
func (w *Watcher) Errors() <-chan error { return w.errs }

// Close releases the underlying fsnotify watcher. After Close, no
// further events or errors will be delivered on the respective channels.
func (w *Watcher) Close() error {
	return w.watcher.Close()
}

func (w *Watcher) pump() {
	for {
		select {
		case ev, ok := <-w.watcher.Events:
			if !ok {
				close(w.events)
				close(w.errs)
				return
			}
			w.handleFSEvent(ev)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				close(w.events)
				close(w.errs)
				return
			}
			if err == nil {
				continue
			}
			select {
			case w.errs <- err:
			default:
				// Drop on floor rather than block pump; caller can't keep up.
			}
		}
	}
}

func (w *Watcher) handleFSEvent(ev fsnotify.Event) {
	// Lazily watch directories created after Add() walked the tree.
	if ev.Has(fsnotify.Create) {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			// Best-effort; if this fails the new directory's contents
			// won't be tracked, but existing watches keep working.
			_ = w.watcher.Add(ev.Name)
		}
	}
	select {
	case w.events <- ports.WatchEvent{Path: ev.Name, Op: translateOp(ev.Op)}:
	default:
		// Drop on floor; consumer is slower than event rate.
	}
}

func translateOp(op fsnotify.Op) ports.WatchOp {
	var out ports.WatchOp
	if op&fsnotify.Create != 0 {
		out |= ports.WatchOpCreate
	}
	if op&fsnotify.Write != 0 {
		out |= ports.WatchOpWrite
	}
	if op&fsnotify.Remove != 0 {
		out |= ports.WatchOpRemove
	}
	if op&fsnotify.Rename != 0 {
		out |= ports.WatchOpRename
	}
	return out
}
