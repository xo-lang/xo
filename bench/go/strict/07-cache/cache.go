// Package benchtask implements a generic in memory loading cache and an HTTP
// service that puts it in front of a slow upstream.
package benchtask

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrLoadTimeout is returned to every waiter when a load exceeds LoadTimeout.
	ErrLoadTimeout = errors.New("cache: load timed out")
	// ErrLoaderPanic is returned to every waiter when the loader panics or
	// exits its goroutine without returning.
	ErrLoaderPanic = errors.New("cache: loader panicked")
	// ErrInvalidOptions is returned by New for unusable configuration.
	ErrInvalidOptions = errors.New("cache: invalid options")
)

// LoaderFunc loads the value for key. The context is canceled when the load
// times out.
type LoaderFunc[K comparable, V any] func(ctx context.Context, key K) (V, error)

// Options configures a Cache.
type Options struct {
	// MaxEntries bounds the number of stored entries (values and cached
	// errors). Zero means unbounded.
	MaxEntries int
	// TTL is how long a value is fresh. Must be positive.
	TTL time.Duration
	// StaleTTL is how long after TTL a value is still served while one
	// background refresh runs.
	StaleTTL time.Duration
	// NegativeTTL is how long loader errors are cached. Zero disables
	// negative caching.
	NegativeTTL time.Duration
	// LoadTimeout bounds a single load. Zero means no timeout.
	LoadTimeout time.Duration
	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time
	// After is used to implement LoadTimeout. Defaults to time.After.
	After func(time.Duration) <-chan time.Time
}

// Metrics is a snapshot of cache counters.
type Metrics struct {
	Hits       uint64
	StaleHits  uint64
	Misses     uint64
	Loads      uint64
	LoadErrors uint64
	Evictions  uint64
}

type entry[K comparable, V any] struct {
	key      K
	val      V
	err      error
	storedAt time.Time
}

// call is one in flight load shared by all its waiters.
type call[V any] struct {
	done        chan struct{}
	val         V
	err         error
	refresh     bool
	invalidated bool
}

type result[V any] struct {
	val V
	err error
	// transient marks timeouts and panics, which are not negatively cached.
	transient bool
}

// Cache is a concurrency safe loading cache with single flight loads,
// stale while revalidate, negative caching, and LRU eviction.
type Cache[K comparable, V any] struct {
	load LoaderFunc[K, V]
	opts Options

	mu       sync.Mutex
	entries  map[K]*list.Element
	lru      *list.List // front is most recently used; values are *entry[K, V]
	inflight map[K]*call[V]
	m        Metrics
}

// New creates a Cache.
func New[K comparable, V any](load LoaderFunc[K, V], opts Options) (*Cache[K, V], error) {
	if load == nil {
		return nil, fmt.Errorf("%w: nil loader", ErrInvalidOptions)
	}
	if opts.TTL <= 0 {
		return nil, fmt.Errorf("%w: TTL must be positive", ErrInvalidOptions)
	}
	if opts.StaleTTL < 0 || opts.NegativeTTL < 0 || opts.LoadTimeout < 0 || opts.MaxEntries < 0 {
		return nil, fmt.Errorf("%w: negative value", ErrInvalidOptions)
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.After == nil {
		opts.After = time.After
	}
	return &Cache[K, V]{
		load:     load,
		opts:     opts,
		entries:  make(map[K]*list.Element),
		lru:      list.New(),
		inflight: make(map[K]*call[V]),
	}, nil
}

// Get returns the value for key, loading it if needed. If ctx ends while
// waiting for a load, Get returns ctx.Err() and the load continues for the
// other waiters.
func (c *Cache[K, V]) Get(ctx context.Context, key K) (V, error) {
	var zero V
	c.mu.Lock()
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*entry[K, V])
		age := c.opts.Now().Sub(e.storedAt)
		switch {
		case e.err != nil && age < c.opts.NegativeTTL:
			c.m.Hits++
			c.lru.MoveToFront(el)
			err := e.err
			c.mu.Unlock()
			return zero, err
		case e.err == nil && age < c.opts.TTL:
			c.m.Hits++
			c.lru.MoveToFront(el)
			v := e.val
			c.mu.Unlock()
			return v, nil
		case e.err == nil && age < c.opts.TTL+c.opts.StaleTTL:
			c.m.StaleHits++
			c.lru.MoveToFront(el)
			if _, busy := c.inflight[key]; !busy {
				c.startLoadLocked(key, true)
			}
			v := e.val
			c.mu.Unlock()
			return v, nil
		default:
			c.removeLocked(el)
		}
	}
	c.m.Misses++
	cl, ok := c.inflight[key]
	if !ok {
		cl = c.startLoadLocked(key, false)
	}
	c.mu.Unlock()

	select {
	case <-cl.done:
		return cl.val, cl.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

// Invalidate removes key. A load in flight for key still completes for its
// current waiters but its result is not stored.
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

func (c *Cache[K, V]) startLoadLocked(key K, refresh bool) *call[V] {
	cl := &call[V]{done: make(chan struct{}), refresh: refresh}
	c.inflight[key] = cl
	c.m.Loads++
	go c.runLoad(key, cl)
	return cl
}

func (c *Cache[K, V]) runLoad(key K, cl *call[V]) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var timeout <-chan time.Time
	if c.opts.LoadTimeout > 0 {
		timeout = c.opts.After(c.opts.LoadTimeout)
	}
	resCh := make(chan result[V], 1)
	go c.callLoader(ctx, key, resCh)

	var r result[V]
	select {
	case r = <-resCh:
	case <-timeout:
		cancel()
		r = result[V]{err: fmt.Errorf("%w after %v", ErrLoadTimeout, c.opts.LoadTimeout), transient: true}
	}
	c.finish(key, cl, r)
}

// callLoader runs the loader and always sends exactly one result, converting
// panics and runtime.Goexit into errors.
func (c *Cache[K, V]) callLoader(ctx context.Context, key K, out chan<- result[V]) {
	returned := false
	defer func() {
		if returned {
			return
		}
		err := fmt.Errorf("%w: loader exited without returning", ErrLoaderPanic)
		if p := recover(); p != nil {
			err = fmt.Errorf("%w: %v", ErrLoaderPanic, p)
		}
		out <- result[V]{err: err, transient: true}
	}()
	v, err := c.load(ctx, key)
	returned = true
	out <- result[V]{val: v, err: err}
}

func (c *Cache[K, V]) finish(key K, cl *call[V], r result[V]) {
	c.mu.Lock()
	if cur, ok := c.inflight[key]; ok && cur == cl {
		delete(c.inflight, key)
	}
	if r.err != nil {
		c.m.LoadErrors++
	}
	if !cl.invalidated {
		switch {
		case r.err == nil:
			c.storeLocked(key, r.val, nil)
		case !cl.refresh && !r.transient && c.opts.NegativeTTL > 0:
			var zero V
			c.storeLocked(key, zero, r.err)
		}
	}
	cl.val, cl.err = r.val, r.err
	c.mu.Unlock()
	close(cl.done)
}

func (c *Cache[K, V]) storeLocked(key K, val V, err error) {
	now := c.opts.Now()
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*entry[K, V])
		e.val, e.err, e.storedAt = val, err, now
		c.lru.MoveToFront(el)
		return
	}
	c.entries[key] = c.lru.PushFront(&entry[K, V]{key: key, val: val, err: err, storedAt: now})
	for c.opts.MaxEntries > 0 && c.lru.Len() > c.opts.MaxEntries {
		back := c.lru.Back()
		if back == nil {
			break
		}
		c.removeLocked(back)
		c.m.Evictions++
	}
}

func (c *Cache[K, V]) removeLocked(el *list.Element) {
	e := el.Value.(*entry[K, V])
	delete(c.entries, e.key)
	c.lru.Remove(el)
}
