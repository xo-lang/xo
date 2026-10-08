package benchtask

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// manualTimers replaces time.After so tests decide when a timeout fires.
type manualTimers struct {
	mu    sync.Mutex
	chans []chan time.Time
}

func (m *manualTimers) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chans = append(m.chans, ch)
	return ch
}

func (m *manualTimers) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.chans)
}

func (m *manualTimers) FireAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ch := range m.chans {
		select {
		case ch <- time.Time{}:
		default:
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func mustNew[K comparable, V any](t *testing.T, load LoaderFunc[K, V], opts Options) *Cache[K, V] {
	t.Helper()
	c, err := New(load, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func mustGet[K comparable, V any](t *testing.T, c *Cache[K, V], key K) V {
	t.Helper()
	v, err := c.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("Get(%v): %v", key, err)
	}
	return v
}

func TestNewValidatesOptions(t *testing.T) {
	load := func(context.Context, string) (int, error) { return 0, nil }
	cases := []struct {
		name string
		load LoaderFunc[string, int]
		opts Options
	}{
		{"nil loader", nil, Options{TTL: time.Second}},
		{"zero ttl", load, Options{}},
		{"negative stale", load, Options{TTL: time.Second, StaleTTL: -1}},
		{"negative max", load, Options{TTL: time.Second, MaxEntries: -1}},
	}
	for _, tc := range cases {
		if _, err := New(tc.load, tc.opts); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("%s: err = %v, want ErrInvalidOptions", tc.name, err)
		}
	}
}

func TestConcurrentGetsShareOneLoad(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	c := mustNew(t, func(context.Context, string) (int, error) {
		calls.Add(1)
		<-release
		return 42, nil
	}, Options{MaxEntries: 10, TTL: time.Minute, Now: newFakeClock().Now})

	const n = 100
	results := make([]int, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			results[i], errs[i] = c.Get(t.Context(), "k")
		})
	}
	waitFor(t, "all callers waiting", func() bool { return c.Metrics().Misses == n })
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
	for i := range n {
		if errs[i] != nil || results[i] != 42 {
			t.Fatalf("caller %d got (%d, %v), want (42, nil)", i, results[i], errs[i])
		}
	}
	if m := c.Metrics(); m.Loads != 1 {
		t.Fatalf("Loads = %d, want 1", m.Loads)
	}
}

func TestTTLExpiryAndStaleWhileRevalidate(t *testing.T) {
	clock := newFakeClock()
	var calls atomic.Int32
	refreshStarted := make(chan struct{}, 1)
	releaseRefresh := make(chan struct{})
	c := mustNew(t, func(context.Context, string) (int, error) {
		n := calls.Add(1)
		if n == 2 {
			refreshStarted <- struct{}{}
			<-releaseRefresh
		}
		return int(n), nil
	}, Options{MaxEntries: 10, TTL: 10 * time.Second, StaleTTL: 20 * time.Second, Now: clock.Now})

	if v := mustGet(t, c, "k"); v != 1 {
		t.Fatalf("first get = %d, want 1", v)
	}
	clock.Advance(5 * time.Second)
	if v := mustGet(t, c, "k"); v != 1 {
		t.Fatalf("fresh get = %d, want 1", v)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls after fresh hit = %d, want 1", got)
	}

	// Age 15s: stale. Every get returns the old value immediately.
	clock.Advance(10 * time.Second)
	for i := range 20 {
		if v := mustGet(t, c, "k"); v != 1 {
			t.Fatalf("stale get %d = %d, want 1", i, v)
		}
	}
	select {
	case <-refreshStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("background refresh never started")
	}
	for range 5 {
		if v := mustGet(t, c, "k"); v != 1 {
			t.Fatalf("stale get during refresh = %d, want 1", v)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls during refresh = %d, want exactly 2", got)
	}
	m := c.Metrics()
	if m.StaleHits != 25 || m.Loads != 2 {
		t.Fatalf("metrics = %+v, want 25 stale hits and 2 loads", m)
	}

	close(releaseRefresh)
	waitFor(t, "refreshed value", func() bool {
		v, err := c.Get(t.Context(), "k")
		return err == nil && v == 2
	})
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls after refresh = %d, want exactly 2", got)
	}

	// Past TTL+StaleTTL since the refresh stored: fully expired, blocking load.
	clock.Advance(31 * time.Second)
	if v := mustGet(t, c, "k"); v != 3 {
		t.Fatalf("get after expiry = %d, want 3", v)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls after expiry = %d, want 3", got)
	}
}

func TestLRUEvictsLeastRecentlyUsed(t *testing.T) {
	var mu sync.Mutex
	loads := map[string]int{}
	c := mustNew(t, func(_ context.Context, k string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		loads[k]++
		return "v-" + k, nil
	}, Options{MaxEntries: 2, TTL: time.Minute, Now: newFakeClock().Now})
	count := func(k string) int {
		mu.Lock()
		defer mu.Unlock()
		return loads[k]
	}

	mustGet(t, c, "a")
	mustGet(t, c, "b")
	mustGet(t, c, "a") // a is now most recently used
	mustGet(t, c, "c") // evicts b

	if m := c.Metrics(); m.Evictions != 1 {
		t.Fatalf("Evictions = %d, want 1", m.Evictions)
	}
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
	if v := mustGet(t, c, "a"); v != "v-a" || count("a") != 1 {
		t.Fatalf("a should still be cached: v=%q loads=%d", v, count("a"))
	}
	mustGet(t, c, "b")
	if count("b") != 2 {
		t.Fatalf("b loads = %d, want 2 (it should have been evicted)", count("b"))
	}
	if count("c") != 1 {
		t.Fatalf("c loads = %d, want 1", count("c"))
	}
}

func TestInvalidateDuringInflightLoad(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	c := mustNew(t, func(context.Context, string) (int, error) {
		n := calls.Add(1)
		if n == 1 {
			started <- struct{}{}
			<-release
		}
		return int(n) * 10, nil
	}, Options{MaxEntries: 10, TTL: time.Minute, Now: newFakeClock().Now})

	type res struct {
		v   int
		err error
	}
	out := make(chan res, 1)
	go func() {
		v, err := c.Get(t.Context(), "k")
		out <- res{v, err}
	}()
	<-started
	c.Invalidate("k")
	close(release)
	r := <-out
	if r.err != nil || r.v != 10 {
		t.Fatalf("waiting caller got (%d, %v), want (10, nil)", r.v, r.err)
	}
	if c.Len() != 0 {
		t.Fatalf("Len = %d, want 0: invalidated load must not be stored", c.Len())
	}
	if v := mustGet(t, c, "k"); v != 20 {
		t.Fatalf("get after invalidate = %d, want 20 (a fresh load)", v)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestNegativeCaching(t *testing.T) {
	clock := newFakeClock()
	errBoom := errors.New("boom")
	var calls atomic.Int32
	var fail atomic.Bool
	fail.Store(true)
	c := mustNew(t, func(context.Context, string) (int, error) {
		calls.Add(1)
		if fail.Load() {
			return 0, errBoom
		}
		return 7, nil
	}, Options{MaxEntries: 10, TTL: time.Minute, NegativeTTL: 5 * time.Second, Now: clock.Now})

	for i := range 3 {
		if _, err := c.Get(t.Context(), "k"); !errors.Is(err, errBoom) {
			t.Fatalf("get %d: err = %v, want boom", i, err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1 (error should be cached)", got)
	}
	fail.Store(false)
	clock.Advance(4 * time.Second)
	if _, err := c.Get(t.Context(), "k"); !errors.Is(err, errBoom) {
		t.Fatalf("within negative ttl: err = %v, want boom", err)
	}
	clock.Advance(2 * time.Second)
	if v := mustGet(t, c, "k"); v != 7 {
		t.Fatalf("after negative ttl = %d, want 7", v)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
	if m := c.Metrics(); m.LoadErrors != 1 {
		t.Fatalf("LoadErrors = %d, want 1", m.LoadErrors)
	}
}

func getConcurrently[V any](t *testing.T, c *Cache[string, V], n int) ([]error, func()) {
	t.Helper()
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			_, errs[i] = c.Get(t.Context(), "k")
		})
	}
	return errs, wg.Wait
}

func TestLoaderTimeoutFailsAllWaiters(t *testing.T) {
	timers := &manualTimers{}
	var calls atomic.Int32
	var hang atomic.Bool
	hang.Store(true)
	canceled := make(chan struct{})
	c := mustNew(t, func(ctx context.Context, _ string) (int, error) {
		calls.Add(1)
		if hang.Load() {
			<-ctx.Done()
			close(canceled)
			return 0, ctx.Err()
		}
		return 5, nil
	}, Options{
		MaxEntries: 10, TTL: time.Minute, NegativeTTL: time.Minute,
		LoadTimeout: time.Second, Now: newFakeClock().Now, After: timers.After,
	})

	const n = 10
	errs, wait := getConcurrently(t, c, n)
	waitFor(t, "all callers waiting", func() bool { return c.Metrics().Misses == n })
	waitFor(t, "timeout armed", func() bool { return timers.Count() == 1 })
	timers.FireAll()
	wait()
	for i, err := range errs {
		if !errors.Is(err, ErrLoadTimeout) {
			t.Fatalf("waiter %d: err = %v, want ErrLoadTimeout", i, err)
		}
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("loader context was not canceled on timeout")
	}

	hang.Store(false)
	if v := mustGet(t, c, "k"); v != 5 {
		t.Fatalf("get after timeout = %d, want 5", v)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestLoaderPanicFailsAllWaiters(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	c := mustNew(t, func(context.Context, string) (int, error) {
		if calls.Add(1) == 1 {
			<-release
			panic("kaboom")
		}
		return 9, nil
	}, Options{MaxEntries: 10, TTL: time.Minute, NegativeTTL: time.Minute, Now: newFakeClock().Now})

	const n = 10
	errs, wait := getConcurrently(t, c, n)
	waitFor(t, "all callers waiting", func() bool { return c.Metrics().Misses == n })
	close(release)
	wait()
	for i, err := range errs {
		if !errors.Is(err, ErrLoaderPanic) {
			t.Fatalf("waiter %d: err = %v, want ErrLoaderPanic", i, err)
		}
	}
	if v := mustGet(t, c, "k"); v != 9 {
		t.Fatalf("get after panic = %d, want 9", v)
	}
	if m := c.Metrics(); m.LoadErrors != 1 || m.Loads != 2 {
		t.Fatalf("metrics = %+v, want 1 load error and 2 loads", m)
	}
}

func TestGetHonorsCallerContext(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	c := mustNew(t, func(context.Context, string) (int, error) {
		<-release
		return 1, nil
	}, Options{TTL: time.Minute})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := c.Get(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
