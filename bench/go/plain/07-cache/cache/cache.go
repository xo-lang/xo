// Package cache implements a generic in memory loading cache with TTL,
// stale while revalidate, negative caching, single flight loads, load
// timeouts, panic isolation and LRU eviction.
package cache

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"
)

// ErrLoadTimeout is returned to every waiter when a load exceeds LoadTimeout.
var ErrLoadTimeout = errors.New("cache: load timed out")

// PanicError is returned to every waiter when the loader panics.
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("cache: loader panicked: %v", e.Value)
}

// Loader loads the value for key. The context is cancelled when the load
// exceeds LoadTimeout.
type Loader[K comparable, V any] func(ctx context.Context, key K) (V, error)

// Options configures a Cache.
type Options struct {
	// MaxEntries is the maximum number of entries (values and cached errors)
	// kept. Must be > 0.
	MaxEntries int
	// TTL is how long an entry is fresh. Must be > 0.
	TTL time.Duration
	// StaleTTL is how long after TTL an entry may still be served while a
	// background refresh runs. Zero disables stale while revalidate.
	StaleTTL time.Duration
	// NegativeTTL is how long loader errors are cached. Zero disables
	// negative caching.
	NegativeTTL time.Duration
	// LoadTimeout bounds each load. Zero means no timeout.
	LoadTimeout time.Duration
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
	// CacheError reports whether a loader error should be negatively cached.
	// Nil means every loader error is cached. Timeouts and panics are never
	// cached regardless of this setting.
	CacheError func(error) bool
}

// Metrics is a snapshot of cache counters.
type Metrics struct {
	Hits       uint64 // fresh entries served (including cached errors)
	StaleHits  uint64 // stale entries served while revalidating
	Misses     uint64 // gets that had to wait for a load
	Loads      uint64 // loader invocations (foreground and background)
	LoadErrors uint64 // loads that failed (error, timeout or panic)
	Evictions  uint64 // entries removed by LRU eviction
}

type entry[K comparable, V any] struct {
	key      K
	val      V
	err      error
	storedAt time.Time
}

type call[V any] struct {
	done        chan struct{}
	val         V
	err         error
	invalidated bool // guarded by Cache.mu
}

// Cache is a generic loading cache. It is safe for concurrent use.
type Cache[K comparable, V any] struct {
	load Loader[K, V]
	opts Options

	mu       sync.Mutex
	entries  map[K]*list.Element // values are *entry[K, V]
	lru      *list.List          // front is most recently used
	inflight map[K]*call[V]
	m        Metrics

	bg sync.WaitGroup // tracks running load coordinators
}

// New creates a Cache.
func New[K comparable, V any](load Loader[K, V], opts Options) (*Cache[K, V], error) {
	if load == nil {
		return nil, errors.New("cache: loader is nil")
	}
	if opts.MaxEntries <= 0 {
		return nil, errors.New("cache: MaxEntries must be > 0")
	}
	if opts.TTL <= 0 {
		return nil, errors.New("cache: TTL must be > 0")
	}
	if opts.StaleTTL < 0 || opts.NegativeTTL < 0 || opts.LoadTimeout < 0 {
		return nil, errors.New("cache: durations must not be negative")
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Cache[K, V]{
		load:     load,
		opts:     opts,
		entries:  make(map[K]*list.Element),
		lru:      list.New(),
		inflight: make(map[K]*call[V]),
	}, nil
}

// Get returns the value for key, loading it if needed. If ctx is done before
// the load completes, Get returns ctx.Err() and the load continues for other
// waiters.
func (c *Cache[K, V]) Get(ctx context.Context, key K) (V, error) {
	c.mu.Lock()
	now := c.opts.Now()
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*entry[K, V])
		age := now.Sub(e.storedAt)
		switch {
		case e.err != nil:
			if age < c.opts.NegativeTTL {
				c.m.Hits++
				c.lru.MoveToFront(el)
				err := e.err
				c.mu.Unlock()
				var zero V
				return zero, err
			}
			c.removeLocked(el)
		case age < c.opts.TTL:
			c.m.Hits++
			c.lru.MoveToFront(el)
			v := e.val
			c.mu.Unlock()
			return v, nil
		case age < c.opts.TTL+c.opts.StaleTTL:
			c.m.StaleHits++
			c.lru.MoveToFront(el)
			v := e.val
			if _, busy := c.inflight[key]; !busy {
				c.startLocked(key)
			}
			c.mu.Unlock()
			return v, nil
		default:
			c.removeLocked(el)
		}
	}
	c.m.Misses++
	cl, ok := c.inflight[key]
	if !ok {
		cl = c.startLocked(key)
	}
	c.mu.Unlock()

	select {
	case <-cl.done:
		return cl.val, cl.err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}

// Invalidate removes key. A load in flight for key will still deliver its
// result to callers already waiting, but the result is not stored.
func (c *Cache[K, V]) Invalidate(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.entries[key]; ok {
		c.removeLocked(el)
	}
	if cl, ok := c.inflight[key]; ok {
		cl.invalidated = true
		delete(c.inflight, key)
	}
}

// Metrics returns a snapshot of the counters.
func (c *Cache[K, V]) Metrics() Metrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m
}

// Len returns the number of stored entries.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

// startLocked registers a new in flight load for key and starts it.
func (c *Cache[K, V]) startLocked(key K) *call[V] {
	cl := &call[V]{done: make(chan struct{})}
	c.inflight[key] = cl
	c.m.Loads++
	c.bg.Add(1)
	go c.runLoad(key, cl)
	return cl
}

func (c *Cache[K, V]) runLoad(key K, cl *call[V]) {
	defer c.bg.Done()

	ctx := context.Background()
	cancel := context.CancelFunc(func() {})
	if c.opts.LoadTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, c.opts.LoadTimeout)
	}
	defer cancel()

	type result struct {
		v   V
		err error
	}
	ch := make(chan result, 1) // buffered so an abandoned loader never blocks
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- result{err: &PanicError{Value: r, Stack: debug.Stack()}}
			}
		}()
		v, err := c.load(ctx, key)
		ch <- result{v: v, err: err}
	}()

	var r result
	select {
	case r = <-ch:
		if r.err != nil && ctx.Err() != nil && errors.Is(r.err, context.DeadlineExceeded) {
			r = result{err: ErrLoadTimeout}
		}
	case <-ctx.Done():
		r = result{err: ErrLoadTimeout}
	}
	c.finish(key, cl, r.v, r.err)
}

func (c *Cache[K, V]) finish(key K, cl *call[V], v V, err error) {
	c.mu.Lock()
	if c.inflight[key] == cl {
		delete(c.inflight, key)
	}
	if err != nil {
		c.m.LoadErrors++
	}
	if !cl.invalidated {
		if err == nil {
			c.storeLocked(key, v, nil)
		} else if c.cacheableLocked(err) {
			var zero V
			c.storeLocked(key, zero, err)
		}
		// Otherwise keep whatever entry exists (a stale value keeps being
		// served until it fully expires).
	}
	cl.val, cl.err = v, err
	c.mu.Unlock()
	close(cl.done)
}

func (c *Cache[K, V]) cacheableLocked(err error) bool {
	if c.opts.NegativeTTL <= 0 {
		return false
	}
	var pe *PanicError
	if errors.Is(err, ErrLoadTimeout) || errors.As(err, &pe) {
		return false
	}
	if c.opts.CacheError != nil {
		return c.opts.CacheError(err)
	}
	return true
}

func (c *Cache[K, V]) storeLocked(key K, v V, err error) {
	now := c.opts.Now()
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*entry[K, V])
		e.val, e.err, e.storedAt = v, err, now
		c.lru.MoveToFront(el)
		return
	}
	el := c.lru.PushFront(&entry[K, V]{key: key, val: v, err: err, storedAt: now})
	c.entries[key] = el
	for c.lru.Len() > c.opts.MaxEntries {
		c.removeLocked(c.lru.Back())
		c.m.Evictions++
	}
}

func (c *Cache[K, V]) removeLocked(el *list.Element) {
	e := c.lru.Remove(el).(*entry[K, V])
	delete(c.entries, e.key)
}
