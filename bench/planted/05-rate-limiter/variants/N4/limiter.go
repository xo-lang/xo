package main

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Default limits applied to keys without a custom configuration.
const (
	DefaultCapacity     = 20
	DefaultRefillPerSec = 5
	DefaultIdleTimeout  = 10 * time.Minute
	DefaultEvictEvery   = time.Minute
)

// Limit describes a token bucket configuration.
type Limit struct {
	Capacity     float64
	RefillPerSec float64
}

// Bucket is a token bucket with fractional tokens.
type Bucket struct {
	Capacity     float64
	Tokens       float64
	RefillPerSec float64
	LastRefill   time.Time
}

// NewBucket returns a full bucket whose refill clock starts at now.
func NewBucket(l Limit, now time.Time) *Bucket {
	return &Bucket{
		Capacity:     l.Capacity,
		Tokens:       l.Capacity,
		RefillPerSec: l.RefillPerSec,
		LastRefill:   now,
	}
}

func (b *Bucket) refill(now time.Time) {
	elapsed := now.Sub(b.LastRefill)
	if elapsed > 0 {
		b.Tokens = math.Min(b.Capacity, b.Tokens+elapsed.Seconds()*b.RefillPerSec)
		b.LastRefill = now
	}
}

// Take refills the bucket based on time elapsed since the last refill and then
// consumes one token if one is available.
func (b *Bucket) Take(now time.Time) bool {
	b.refill(now)
	if b.Tokens >= 1 {
		b.Tokens--
		return true
	}
	return false
}

// RetryAfter returns the whole number of seconds until one token is
// available, never less than 1. It does not mutate the bucket beyond a
// refill.
func (b *Bucket) RetryAfter(now time.Time) int {
	b.refill(now)
	if b.Tokens >= 1 {
		return 1
	}
	if b.RefillPerSec <= 0 {
		// The bucket never refills; report a large but finite value.
		return math.MaxInt32
	}
	secs := (1 - b.Tokens) / b.RefillPerSec
	// Tolerate float noise so 2.0000000001 does not become 3.
	n := int(math.Ceil(secs - 1e-9))
	if n < 1 {
		n = 1
	}
	return n
}

// Limiter keeps one bucket per client key. It is safe for concurrent use.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*Bucket
	def     Limit
	custom  map[string]Limit
	now     func() time.Time
	idle    time.Duration
}

// Option configures a Limiter.
type Option func(*Limiter)

// WithClock injects a clock, used by tests.
func WithClock(now func() time.Time) Option { return func(l *Limiter) { l.now = now } }

// WithDefault overrides the default limit.
func WithDefault(lim Limit) Option { return func(l *Limiter) { l.def = lim } }

// WithIdleTimeout overrides how long a bucket may sit unused before eviction.
func WithIdleTimeout(d time.Duration) Option { return func(l *Limiter) { l.idle = d } }

// NewLimiter builds a limiter. custom maps specific keys to their own limits.
func NewLimiter(custom map[string]Limit, opts ...Option) *Limiter {
	l := &Limiter{
		buckets: make(map[string]*Bucket),
		def:     Limit{Capacity: DefaultCapacity, RefillPerSec: DefaultRefillPerSec},
		custom:  make(map[string]Limit, len(custom)),
		now:     time.Now,
		idle:    DefaultIdleTimeout,
	}
	for k, v := range custom {
		l.custom[k] = v
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Allow consumes a token for key. When denied it also returns the
// Retry-After value in whole seconds.
func (l *Limiter) Allow(key string) (ok bool, retryAfter int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b, found := l.buckets[key]
	if !found {
		lim, has := l.custom[key]
		if !has {
			lim = l.def
		}
		b = NewBucket(lim, now)
		l.buckets[key] = b
	}
	if b.Take(now) {
		return true, 0
	}
	return false, b.RetryAfter(now)
}

// Len returns the number of tracked buckets.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// Evict removes buckets idle for longer than the idle timeout and returns how
// many were removed. A bucket's last refill time is updated on every request,
// so it doubles as the last-seen time.
func (l *Limiter) Evict() int {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for k, b := range l.buckets {
		if now.Sub(b.LastRefill) < l.idle {
			delete(l.buckets, k)
			n++
		}
	}
	return n
}

// RunEvictor evicts idle buckets every interval until ctx is cancelled. The
// returned channel is closed once the goroutine has exited.
func (l *Limiter) RunEvictor(ctx context.Context, every time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				l.Evict()
			}
		}
	}()
	return done
}

// ClientKey returns the X-Api-Key header, or the remote IP if it is missing.
func ClientKey(r *http.Request) string {
	if k := r.Header.Get("X-Api-Key"); k != "" {
		return k
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Middleware rejects requests over the limit with 429 and Retry-After.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retry := l.Allow(ClientKey(r))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
