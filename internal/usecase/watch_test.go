package usecase

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/techgodhq/creed/internal/ports"
)

type fakeWatcher struct {
	mu       sync.Mutex
	added    []string
	events   chan ports.WatchEvent
	errs     chan error
	closed   atomic.Bool
	addErr   error
	closeErr error
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{
		events: make(chan ports.WatchEvent, 16),
		errs:   make(chan error, 16),
	}
}

func (f *fakeWatcher) Add(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, path)
	return nil
}

func (f *fakeWatcher) Events() <-chan ports.WatchEvent { return f.events }
func (f *fakeWatcher) Errors() <-chan error            { return f.errs }

func (f *fakeWatcher) Close() error {
	f.closed.Store(true)
	return f.closeErr
}

func TestWatchEngineReturnsContextErrorImmediatelyWhenContextCancelled(t *testing.T) {
	w := newFakeWatcher()
	var syncCalls int32
	engine := NewWatchEngine(w, func(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
		atomic.AddInt32(&syncCalls, 1)
		return &SyncResult{}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := engine.Watch(ctx, []string{"./.creed"}, WatchOptions{Debounce: 10 * time.Millisecond}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch returned %v, want context.Canceled", err)
	}
	if atomic.LoadInt32(&syncCalls) != 0 {
		t.Fatalf("sync called %d times on cancelled context", syncCalls)
	}
}

func TestWatchEngineBurstsProduceSingleDebouncedSync(t *testing.T) {
	w := newFakeWatcher()
	var (
		mu        sync.Mutex
		summaries []WatchSummary
	)
	syncCounter := atomic.Int32{}
	engine := NewWatchEngine(w, func(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
		syncCounter.Add(1)
		return &SyncResult{Targets: []TargetResult{{Target: opts.Target}}}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sink := func(s WatchSummary) {
		mu.Lock()
		summaries = append(summaries, s)
		mu.Unlock()
	}

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- engine.Watch(ctx, []string{"./.creed"}, WatchOptions{Debounce: 25 * time.Millisecond}, sink)
	}()

	// Fire three rapid events that should collapse into one sync.
	for i := 0; i < 3; i++ {
		w.events <- ports.WatchEvent{Path: "./.creed/config/project.md", Op: ports.WatchOpWrite}
	}

	waitFor(t, func() bool {
		return syncCounter.Load() == 1
	}, "expected exactly one sync after a burst", 1*time.Second)

	// Give the debounce a bit more time to catch any second-trigger that
	// would indicate a bug.
	time.Sleep(80 * time.Millisecond)

	if got := syncCounter.Load(); got != 1 {
		t.Fatalf("burst produced %d syncs, want 1", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(summaries) != 1 {
		t.Fatalf("sink received %d summaries, want 1", len(summaries))
	}
	if summaries[0].Err != nil {
		t.Fatalf("summary error = %v, want nil", summaries[0].Err)
	}
	if summaries[0].Result == nil {
		t.Fatal("summary Result is nil")
	}
	cancel()
	<-doneCh
}

func TestWatchEngineBatchesSeparateBursts(t *testing.T) {
	w := newFakeWatcher()
	syncCounter := atomic.Int32{}
	engine := NewWatchEngine(w, func(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
		syncCounter.Add(1)
		return &SyncResult{}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- engine.Watch(ctx, []string{"./.creed"}, WatchOptions{Debounce: 25 * time.Millisecond}, nil)
	}()

	// First burst.
	w.events <- ports.WatchEvent{Path: "./.creed/manifest.yaml"}
	waitFor(t, func() bool { return syncCounter.Load() == 1 }, "first burst did not trigger sync", 1*time.Second)

	// Wait past the debounce window, then trigger a second burst.
	time.Sleep(80 * time.Millisecond)
	w.events <- ports.WatchEvent{Path: "./.creed/skills/review.md"}
	waitFor(t, func() bool { return syncCounter.Load() == 2 }, "second burst did not trigger sync", 1*time.Second)

	cancel()
	<-doneCh

	if got := syncCounter.Load(); got != 2 {
		t.Fatalf("saw %d syncs, want 2", got)
	}
}

func TestWatchEngineHaltsOnWatcherError(t *testing.T) {
	w := newFakeWatcher()
	engine := NewWatchEngine(w, func(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
		return &SyncResult{}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.errs <- errors.New("kernel watcher exploded")

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- engine.Watch(ctx, []string{"./.creed"}, WatchOptions{Debounce: 10 * time.Millisecond}, nil)
	}()

	select {
	case err := <-doneCh:
		if err == nil {
			t.Fatal("expected non-nil error from watcher failure")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("watch did not return after watcher error")
	}
}

func TestWatchEngineAddFailureStopsBeforeWatching(t *testing.T) {
	w := newFakeWatcher()
	w.addErr = errors.New("permission denied")
	engine := NewWatchEngine(w, func(ctx context.Context, opts SyncOptions) (*SyncResult, error) {
		t.Fatal("sync must not be called when Add fails")
		return nil, nil
	})

	err := engine.Watch(context.Background(), []string{"./.creed"}, WatchOptions{}, nil)
	if err == nil || err.Error() == "" {
		t.Fatalf("expected error from failed Add, got %v", err)
	}
}

func TestNormalizedDebounce(t *testing.T) {
	cases := []struct {
		in, want time.Duration
	}{
		{0, DefaultWatchDebounce},
		{-1 * time.Second, DefaultWatchDebounce},
		{10 * time.Millisecond, MinWatchDebounce},
		{100 * time.Millisecond, 100 * time.Millisecond},
		{DefaultWatchDebounce, DefaultWatchDebounce},
	}
	for _, tc := range cases {
		if got := normalizedDebounce(tc.in); got != tc.want {
			t.Errorf("normalizedDebounce(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func waitFor(t *testing.T, condition func() bool, msg string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", timeout, msg)
}
