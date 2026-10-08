package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
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

func mustNew[K comparable, V any](t *testing.T, load Loader[K, V], opts Options) *Cache[K, V] {
	t.Helper()
	c, err := New(load, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(c.bg.Wait)
	return c
}

func mustGet[K comparable, V comparable](t *testing.T, c *Cache[K, V], key K, want V) {
	t.Helper()
	got, err := c.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("Get(%v): unexpected error %v", key, err)
	}
	if got != want {
		t.Fatalf("Get(%v) = %v, want %v", key, got, want)
	}
}

func TestNewValidates(t *testing.T) {
	load := func(context.Context, string) (int, error) { return 0, nil }
	cases := []Options{
		{MaxEntries: 0, TTL: time.Second},
		{MaxEntries: 1, TTL: 0},
		{MaxEntries: 1, TTL: time.Second, StaleTTL: -1},
	}
	for i, o := range cases {
		if _, err := New(load, o); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
	if _, err := New[string, int](nil, Options{MaxEntries: 1, TTL: time.Second}); err == nil {
		t.Error("nil loader: expected error")
	}
}

func TestConcurrentGetsShareOneLoad(t *testing.T) {
	clk := newFakeClock()
	var calls atomic.Int32
	gate := make(chan struct{})
	c := mustNew(t, func(ctx context.Context, k string) (int, error) {
		calls.Add(1)
		<-gate
		return 42, nil
	}, Options{MaxEntries: 10, TTL: time.Minute, Now: clk.Now})

	const n = 100
	results := make([]int, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { results[i], errs[i] = c.Get(context.Background(), "k") })
	}
	waitFor(t, "all gets to join the load", func() bool { return c.Metrics().Misses == n })
	close(gate)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("loader called %d times, want 1", got)
	}
	for i := range n {
		if errs[i] != nil || results[i] != 42 {
			t.Fatalf("get %d = (%d, %v), want (42, nil)", i, results[i], errs[i])
		}
	}
	m := c.Metrics()
	if m.Loads != 1 || m.Misses != n {
		t.Fatalf("metrics = %+v", m)
	}
	mustGet(t, c, "k", 42)
	if m := c.Metrics(); m.Hits != 1 {
		t.Fatalf("hits = %d, want 1", m.Hits)
	}
}

func TestTTLAndStaleWhileRevalidate(t *testing.T) {
	clk := newFakeClock()
	var calls atomic.Int32
	gate := make(chan struct{})
	c := mustNew(t, func(ctx context.Context, k string) (string, error) {
		n := calls.Add(1)
		if n == 2 {
			<-gate
		}
		return fmt.Sprintf("v%d", n), nil
	}, Options{MaxEntries: 10, TTL: 10 * time.Second, StaleTTL: 10 * time.Second, Now: clk.Now})

	mustGet(t, c, "k", "v1")

	clk.Advance(5 * time.Second) // age 5s: fresh
	mustGet(t, c, "k", "v1")
	if m := c.Metrics(); m.Hits != 1 || m.Loads != 1 {
		t.Fatalf("after fresh hit: %+v", m)
	}

	clk.Advance(6 * time.Second) // age 11s: stale
	var wg sync.WaitGroup
	const n = 50
	vals := make([]string, n)
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() { vals[i], errs[i] = c.Get(context.Background(), "k") })
	}
	wg.Wait() // stale gets return immediately even though the refresh is blocked
	for i := range n {
		if errs[i] != nil || vals[i] != "v1" {
			t.Fatalf("stale get %d = (%q, %v)", i, vals[i], errs[i])
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("loader called %d times, want 2 (exactly one background refresh)", got)
	}
	if m := c.Metrics(); m.StaleHits != n || m.Loads != 2 {
		t.Fatalf("after stale hits: %+v", m)
	}

	close(gate)
	c.bg.Wait()
	if got := calls.Load(); got != 2 {
		t.Fatalf("loader called %d times after refresh, want 2", got)
	}
	mustGet(t, c, "k", "v2") // refreshed value stored at t0+11s, fresh

	clk.Advance(15 * time.Second) // age 15s since refresh: stale again
	mustGet(t, c, "k", "v2")
	c.bg.Wait()
	mustGet(t, c, "k", "v3")

	clk.Advance(25 * time.Second) // age 25s > TTL+StaleTTL: expired
	missesBefore := c.Metrics().Misses
	mustGet(t, c, "k", "v4") // synchronous load, not stale value
	if m := c.Metrics(); m.Misses != missesBefore+1 {
		t.Fatalf("expired get should be a miss: %+v", m)
	}
}

func TestTTLExpiryWithoutStale(t *testing.T) {
	clk := newFakeClock()
	var calls atomic.Int32
	c := mustNew(t, func(ctx context.Context, k string) (int32, error) {
		return calls.Add(1), nil
	}, Options{MaxEntries: 10, TTL: 10 * time.Second, Now: clk.Now})

	mustGet(t, c, "k", int32(1))
	clk.Advance(9 * time.Second)
	mustGet(t, c, "k", int32(1))
	clk.Advance(time.Second) // exactly TTL: expired
	mustGet(t, c, "k", int32(2))
	if m := c.Metrics(); m.StaleHits != 0 || m.Loads != 2 || m.Misses != 2 || m.Hits != 1 {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestLRUEviction(t *testing.T) {
	clk := newFakeClock()
	var calls atomic.Int32
	c := mustNew(t, func(ctx context.Context, k string) (string, error) {
		calls.Add(1)
		return "val-" + k, nil
	}, Options{MaxEntries: 2, TTL: time.Minute, Now: clk.Now})

	mustGet(t, c, "a", "val-a")
	mustGet(t, c, "b", "val-b")
	mustGet(t, c, "a", "val-a") // a is now most recently used
	mustGet(t, c, "c", "val-c") // evicts b
	if c.Len() != 2 {
		t.Fatalf("Len = %d, want 2", c.Len())
	}
	if m := c.Metrics(); m.Evictions != 1 {
		t.Fatalf("evictions = %d, want 1", m.Evictions)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}
	mustGet(t, c, "a", "val-a") // still cached
	mustGet(t, c, "c", "val-c") // still cached
	if got := calls.Load(); got != 3 {
		t.Fatalf("a and c should be cached; calls = %d", got)
	}
	mustGet(t, c, "b", "val-b") // reloaded, evicts a (LRU)
	if got := calls.Load(); got != 4 {
		t.Fatalf("b should have been reloaded; calls = %d", got)
	}
	mustGet(t, c, "c", "val-c")
	if got := calls.Load(); got != 4 {
		t.Fatalf("c should still be cached; calls = %d", got)
	}
	mustGet(t, c, "a", "val-a")
	if got := calls.Load(); got != 5 {
		t.Fatalf("a should have been evicted; calls = %d", got)
	}
	if m := c.Metrics(); m.Evictions != 3 {
		t.Fatalf("evictions = %d, want 3", m.Evictions)
	}
}

func TestInvalidateDuringInFlightLoad(t *testing.T) {
	clk := newFakeClock()
	var calls atomic.Int32
	started := make(chan struct{})
	gate := make(chan struct{})
	c := mustNew(t, func(ctx context.Context, k string) (int32, error) {
		n := calls.Add(1)
		if n == 1 {
			close(started)
			<-gate
		}
		return n, nil
	}, Options{MaxEntries: 10, TTL: time.Minute, Now: clk.Now})

	type res struct {
		v   int32
		err error
	}
	first := make(chan res, 1)
	go func() {
		v, err := c.Get(context.Background(), "k")
		first <- res{v, err}
	}()
	<-started
	c.Invalidate("k")

	// A get after invalidation must not join the stale in flight load.
	mustGet(t, c, "k", int32(2))

	close(gate)
	r := <-first
	if r.err != nil || r.v != 1 {
		t.Fatalf("waiter got (%d, %v), want (1, nil)", r.v, r.err)
	}
	c.bg.Wait()

	// The invalidated load's result must not overwrite the newer value.
	mustGet(t, c, "k", int32(2))
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestInvalidateDuringInFlightLoadNotStored(t *testing.T) {
	clk := newFakeClock()
	var calls atomic.Int32
	started := make(chan struct{})
	gate := make(chan struct{})
	c := mustNew(t, func(ctx context.Context, k string) (int32, error) {
		n := calls.Add(1)
		if n == 1 {
			close(started)
			<-gate
		}
		return n, nil
	}, Options{MaxEntries: 10, TTL: time.Minute, Now: clk.Now})

	done := make(chan int32, 1)
	go func() {
		v, _ := c.Get(context.Background(), "k")
		done <- v
	}()
	<-started
	c.Invalidate("k")
	close(gate)
	if v := <-done; v != 1 {
		t.Fatalf("waiter got %d, want 1", v)
	}
	c.bg.Wait()
	if c.Len() != 0 {
		t.Fatalf("invalidated result was stored; Len = %d", c.Len())
	}
	mustGet(t, c, "k", int32(2))
}

func TestInvalidateRemovesEntry(t *testing.T) {
	clk := newFakeClock()
	var calls atomic.Int32
	c := mustNew(t, func(ctx context.Context, k string) (int32, error) {
		return calls.Add(1), nil
	}, Options{MaxEntries: 10, TTL: time.Minute, Now: clk.Now})
	mustGet(t, c, "k", int32(1))
	c.Invalidate("k")
	c.Invalidate("missing") // no-op
	mustGet(t, c, "k", int32(2))
}

var errBoom = errors.New("boom")

func TestNegativeCaching(t *testing.T) {
	clk := newFakeClock()
	var calls atomic.Int32
	c := mustNew(t, func(ctx context.Context, k string) (int, error) {
		calls.Add(1)
		return 0, errBoom
	}, Options{MaxEntries: 10, TTL: time.Minute, NegativeTTL: 5 * time.Second, Now: clk.Now})

	for i := range 3 {
		if _, err := c.Get(context.Background(), "k"); !errors.Is(err, errBoom) {
			t.Fatalf("get %d: err = %v, want errBoom", i, err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
	if m := c.Metrics(); m.LoadErrors != 1 || m.Hits != 2 || m.Misses != 1 {
		t.Fatalf("metrics = %+v", m)
	}

	clk.Advance(5 * time.Second) // negative entry expired
	if _, err := c.Get(context.Background(), "k"); !errors.Is(err, errBoom) {
		t.Fatalf("err = %v, want errBoom", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
}

func TestNegativeCachingRespectsCacheErrorAndZeroTTL(t *testing.T) {
	for name, opts := range map[string]Options{
		"predicate": {MaxEntries: 10, TTL: time.Minute, NegativeTTL: time.Minute,
			CacheError: func(err error) bool { return false }},
		"zero ttl": {MaxEntries: 10, TTL: time.Minute},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			c := mustNew(t, func(ctx context.Context, k string) (int, error) {
				calls.Add(1)
				return 0, errBoom
			}, opts)
			for range 3 {
				if _, err := c.Get(context.Background(), "k"); !errors.Is(err, errBoom) {
					t.Fatalf("err = %v", err)
				}
			}
			if got := calls.Load(); got != 3 {
				t.Fatalf("calls = %d, want 3", got)
			}
		})
	}
}

func TestLoaderTimeoutFailsAllWaiters(t *testing.T) {
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	var broken atomic.Bool
	broken.Store(true)
	var calls atomic.Int32
	c, err := New(func(ctx context.Context, k string) (int, error) {
		calls.Add(1)
		if broken.Load() {
			<-hang // ignores ctx on purpose
			return 0, nil
		}
		return 7, nil
	}, Options{MaxEntries: 10, TTL: time.Minute, NegativeTTL: time.Minute, LoadTimeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	const n = 10
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { _, errs[i] = c.Get(context.Background(), "k") })
	}
	wg.Wait()
	for i, err := range errs {
		if !errors.Is(err, ErrLoadTimeout) {
			t.Fatalf("waiter %d: err = %v, want ErrLoadTimeout", i, err)
		}
	}
	if m := c.Metrics(); m.LoadErrors == 0 || m.LoadErrors != m.Loads {
		t.Fatalf("metrics = %+v", m)
	}

	broken.Store(false)
	mustGet(t, c, "k", 7) // timeout was not negatively cached
	mustGet(t, c, "other", 7)
	c.bg.Wait()
}

func TestLoaderRespectingContextTimesOut(t *testing.T) {
	c := mustNew(t, func(ctx context.Context, k string) (int, error) {
		<-ctx.Done()
		return 0, ctx.Err()
	}, Options{MaxEntries: 10, TTL: time.Minute, LoadTimeout: 20 * time.Millisecond})
	if _, err := c.Get(context.Background(), "k"); !errors.Is(err, ErrLoadTimeout) {
		t.Fatalf("err = %v, want ErrLoadTimeout", err)
	}
}

func TestLoaderPanicFailsAllWaiters(t *testing.T) {
	clk := newFakeClock()
	var calls atomic.Int32
	gate := make(chan struct{})
	c := mustNew(t, func(ctx context.Context, k string) (int32, error) {
		n := calls.Add(1)
		if n == 1 {
			<-gate
			panic("loader exploded")
		}
		return n, nil
	}, Options{MaxEntries: 10, TTL: time.Minute, NegativeTTL: time.Minute, Now: clk.Now})

	const n = 10
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { _, errs[i] = c.Get(context.Background(), "k") })
	}
	waitFor(t, "all gets to join the load", func() bool { return c.Metrics().Misses == n })
	close(gate)
	wg.Wait()
	for i, err := range errs {
		var pe *PanicError
		if !errors.As(err, &pe) || pe.Value != "loader exploded" {
			t.Fatalf("waiter %d: err = %v, want PanicError", i, err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}

	mustGet(t, c, "k", int32(2))
	mustGet(t, c, "k", int32(2))
	if m := c.Metrics(); m.LoadErrors != 1 || m.Loads != 2 {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestGetContextCancelled(t *testing.T) {
	gate := make(chan struct{})
	c := mustNew(t, func(ctx context.Context, k string) (int, error) {
		<-gate
		return 1, nil
	}, Options{MaxEntries: 10, TTL: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Get(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	close(gate)
	c.bg.Wait()
	mustGet(t, c, "k", 1) // load still completed and was stored
	if m := c.Metrics(); m.Loads != 1 {
		t.Fatalf("loads = %d, want 1", m.Loads)
	}
}
