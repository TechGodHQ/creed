package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/techgodhq/creed/internal/ports"
)

// WatchOptions controls the behavior of a watch operation.
type WatchOptions struct {
	// Target filters syncs to a single target by name. When empty, all
	// enabled targets in the manifest are synced on each change.
	Target string `json:"target,omitempty"`

	// Quiet suppresses per-target sync result output, reporting only
	// errors and a minimal heartbeat. Honored by sinks that the CLI
	// installs; the engine itself does not interpret this flag.
	Quiet bool `json:"quiet,omitempty"`

	// Force rewrites files even when content is unchanged on each sync.
	Force bool `json:"force,omitempty"`

	// Debounce is the quiet period required after the last filesystem
	// event before a sync is triggered. Each new event received during
	// the window resets the timer, so a continuous burst produces one
	// sync after activity settles, not one sync after the first event.
	// Zero falls back to DefaultWatchDebounce. Values below
	// MinWatchDebounce are clamped up to MinWatchDebounce.
	Debounce time.Duration `json:"debounce,omitempty"`
}

// DefaultWatchDebounce is applied when WatchOptions.Debounce is zero.
const DefaultWatchDebounce = 500 * time.Millisecond

// MinWatchDebounce is the floor for clamped debounce values. Spinning
// faster than this risks emitting partial writes for editors that save
// via rename or multi-step temp swaps.
const MinWatchDebounce = 50 * time.Millisecond

// WatchSink receives per-sync summaries from a running WatchEngine.
// The sink is invoked synchronously from the watch loop; a slow sink
// delays subsequent event processing but cannot halt the engine.
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
// error is always either ctx.Err() (which the CLI maps to a clean
// exit when caused by SIGINT/SIGTERM) or the first watcher-level
// error that cannot be tolerated.
//
// The debounce timer is reset on every new event, so a continuous
// burst produces exactly one sync once activity settles for the
// debounce window. The loop owns all timer state, so there are no
// goroutines to leak on exit.
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
		pending     []string
		timer       *time.Timer
		timerFiredC <-chan time.Time
	)

	// stopTimer stops any active timer and nils it out.
	stopTimer := func() {
		if timer != nil {
			if !timer.Stop() {
				// Drain if it already fired and we hadn't read it.
				select {
				case <-timer.C:
				default:
				}
			}
			timer = nil
			timerFiredC = nil
		}
	}
	defer stopTimer()

	// resetTimer starts (or restarts) the debounce window. Called
	// whenever a new event arrives, even mid-window, so the timer
	// reflects the most recent change.
	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(debounce)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(debounce)
		}
		timerFiredC = timer.C
	}

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
			pending = append(pending, ev.Path)
			resetTimer()
		case <-timerFiredC:
			timer = nil
			timerFiredC = nil
			snapshot := pending
			pending = nil
			if len(snapshot) == 0 {
				continue
			}
			e.runOneSync(ctx, opts, snapshot, sink)
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

func normalizedDebounce(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultWatchDebounce
	}
	if d < MinWatchDebounce {
		return MinWatchDebounce
	}
	return d
}

// EffectiveDebounce returns the debounce duration the engine will use
// for the given option, after applying defaults and clamps. The CLI
// uses this to print an accurate banner.
func EffectiveDebounce(d time.Duration) time.Duration {
	return normalizedDebounce(d)
}
