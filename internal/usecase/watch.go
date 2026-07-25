package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/techgodhq/creed/internal/ports"
)

// WatchOptions controls the behavior of a watch operation.
type WatchOptions struct {
	// Target filters syncs to a single target by name. When empty, all
	// enabled targets in the manifest are synced on each change.
	Target string `json:"target,omitempty"`

	// Quiet suppresses per-target sync result output, reporting only
	// errors and a minimal heartbeat.
	Quiet bool `json:"quiet,omitempty"`

	// Force rewrites files even when content is unchanged on each sync.
	Force bool `json:"force,omitempty"`

	// Debounce is the quiet period required after the last filesystem
	// event before a sync is triggered. Zero falls back to a sane
	// default (DefaultWatchDebounce). Values below MinWatchDebounce are
	// clamped up to MinWatchDebounce.
	Debounce time.Duration `json:"debounce,omitempty"`
}

// DefaultWatchDebounce is applied when WatchOptions.Debounce is zero.
const DefaultWatchDebounce = 500 * time.Millisecond

// MinWatchDebounce is the floor for clamped debounce values. Spinning
// faster than this risks emitting partial writes for editors that save
// via rename or multi-step temp swaps.
const MinWatchDebounce = 50 * time.Millisecond

// WatchSink receives per-sync summaries from a running WatchEngine.
// Returning an error halts the engine.
type WatchSink func(summary WatchSummary)

// WatchSummary is the result of one debounced sync during a watch run.
type WatchSummary struct {
	// TriggeredAt is when the sync was kicked off.
	TriggeredAt time.Time
	// Sources is the list of paths whose changes triggered this sync.
	Sources []string
	// Result is the underlying SyncResult. May be nil when the sync
	// itself failed before producing a result.
	Result *SyncResult
	// Err is any error encountered producing this summary.
	Err error
}

// WatchEngine debounces filesystem events and invokes Sync on each
// stable change. It is the long-running variant of the Sync use case.
type WatchEngine struct {
	watcher ports.Watcher
	sync    func(ctx context.Context, opts SyncOptions) (*SyncResult, error)
	now     func() time.Time
}

// NewWatchEngine constructs a WatchEngine. The sync callback is
// typically a closure over a SyncEngine.Sync invocation.
func NewWatchEngine(watcher ports.Watcher, sync func(ctx context.Context, opts SyncOptions) (*SyncResult, error)) *WatchEngine {
	return &WatchEngine{
		watcher: watcher,
		sync:    sync,
		now:     time.Now,
	}
}

// Watch registers the supplied roots, then debounces events and runs a
// sync for each stable change until ctx is cancelled. The returned
// error is always either ctx.Err() or the first watcher-level error
// that cannot be tolerated.
func (e *WatchEngine) Watch(ctx context.Context, roots []string, opts WatchOptions, sink WatchSink) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	debounce := normalizedDebounce(opts.Debounce)

	for _, root := range roots {
		if err := e.watcher.Add(root); err != nil {
			return fmt.Errorf("watch root %q: %w", root, err)
		}
	}

	var (
		timerMu sync.Mutex
		timer   *time.Timer

		pendingMu sync.Mutex
		pending   []string

		flush   = make(chan struct{}, 1)
		stopCh  = make(chan struct{})
		stopped sync.Once
	)
	defer stopped.Do(func() { close(stopCh) })

	// Goroutine: own the timer so the select below never races on
	// timer.Stop / timer.Reset against itself.
	go func() {
		for {
			select {
			case <-stopCh:
				timerMu.Lock()
				if timer != nil {
					timer.Stop()
				}
				timerMu.Unlock()
				return
			case <-flush:
				// Wait for the debounce window. If a new event arrives
				// during the wait, flush fires again and resets us.
				timerMu.Lock()
				if timer != nil {
					timer.Stop()
				}
				timer = time.NewTimer(debounce)
				t := timer
				timerMu.Unlock()

				select {
				case <-stopCh:
					t.Stop()
					return
				case <-t.C:
					pendingMu.Lock()
					snapshot := drainSources(&pending)
					pendingMu.Unlock()
					if len(snapshot) == 0 {
						continue
					}
					e.runOneSync(ctx, opts, snapshot, sink)
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err, ok := <-e.watcher.Errors():
			if !ok {
				return nil
			}
			if err == nil {
				continue
			}
			return fmt.Errorf("watcher error: %w", err)
		case ev, ok := <-e.watcher.Events():
			if !ok {
				return nil
			}
			pendingMu.Lock()
			pending = append(pending, ev.Path)
			pendingMu.Unlock()
			select {
			case flush <- struct{}{}:
			default:
			}
		}
	}
}

func (e *WatchEngine) runOneSync(ctx context.Context, opts WatchOptions, sources []string, sink WatchSink) {
	started := e.now()
	result, err := e.sync(ctx, SyncOptions{Target: opts.Target, DryRun: false, Force: opts.Force})
	if sink != nil {
		sink(WatchSummary{
			TriggeredAt: started,
			Sources:     sources,
			Result:      result,
			Err:         err,
		})
	}
}

func drainSources(pending *[]string) []string {
	if len(*pending) == 0 {
		return nil
	}
	snapshot := make([]string, len(*pending))
	copy(snapshot, *pending)
	*pending = (*pending)[:0]
	return snapshot
}

func normalizedDebounce(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultWatchDebounce
	}
	if d < MinWatchDebounce {
		return MinWatchDebounce
	}
	return d
}
