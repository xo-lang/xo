package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Default limits and eviction settings.
const (
	DefaultCapacity     = 20
	DefaultRefillPerSec = 5
	IdleTimeout         = 10 * time.Minute
	EvictInterval       = time.Minute
)

// Limit describes the capacity and refill rate of a bucket.
type Limit struct {
	Capacity     float64
	RefillPerSec float64
}

func (l Limit) validate() error {
	if l.Capacity < 1 || math.IsNaN(l.Capacity) || math.IsInf(l.Capacity, 0) {
		return fmt.Errorf("capacity must be a finite number >= 1, got %v", l.Capacity)
	}
	if !(l.RefillPerSec > 0) || math.IsInf(l.RefillPerSec, 0) {
		return fmt.Errorf("refill per second must be a finite number > 0, got %v", l.RefillPerSec)
	}
	return nil
}

// Bucket is a token bucket. It is not safe for concurrent use on its own;
// Limiter guards every bucket with its mutex.
type Bucket struct {
	Capacity     float64
	Tokens       float64
	RefillPerSec float64
	LastRefill   time.Time
}

// NewBucket returns a full bucket for the given limit.
func NewBucket(l Limit, now time.Time) *Bucket {
	return &Bucket{
		Capacity:     l.Capacity,
		Tokens:       l.Capacity,
		RefillPerSec: l.RefillPerSec,
		LastRefill:   now,
	}
}

func (b *Bucket) refill(now time.Time) {
	elapsed := now.Sub(b.LastRefill).Seconds()
	if elapsed <= 0 {
		// Clock did not move forward (or went backwards): add nothing and keep
		// the later timestamp so tokens are never granted twice.
		return
	}
	b.Tokens = math.Min(b.Capacity, b.Tokens+elapsed*b.RefillPerSec)
	b.LastRefill = now
}

// Take refills the bucket based on elapsed time and then consumes one token
// if available. It reports whether a token was consumed.
func (b *Bucket) Take(now time.Time) bool {
	b.refill(now)
	if b.Tokens >= 1 {
		b.Tokens--
		return true
	}
	return false
}

// RetryAfter returns the whole number of seconds until one token is
// available, at least 1. It assumes refill has already been applied for now.
func (b *Bucket) RetryAfter() int {
	need := 1 - b.Tokens
	if need <= 0 || b.RefillPerSec <= 0 {
		return 1
	}
	// Small epsilon so float noise like 2.0000000001 does not round up to 3.
	secs := math.Ceil(need/b.RefillPerSec - 1e-9)
	if secs < 1 {
		return 1
	}
	if secs > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(secs)
}

type entry struct {
	bucket   *Bucket
	lastSeen time.Time
}

// Limiter keeps one bucket per client key. It is safe for concurrent use.
type Limiter struct {
	now    func() time.Time
	def    Limit
	custom map[string]Limit

	mu      sync.Mutex
	buckets map[string]*entry
}

// NewLimiter builds a limiter. now may be nil to use time.Now. custom may be
// nil. All limits are validated.
func NewLimiter(def Limit, custom map[string]Limit, now func() time.Time) (*Limiter, error) {
	if err := def.validate(); err != nil {
		return nil, fmt.Errorf("default limit: %w", err)
	}
	cp := make(map[string]Limit, len(custom))
	for k, v := range custom {
		if err := v.validate(); err != nil {
			return nil, fmt.Errorf("limit for key %q: %w", k, err)
		}
		cp[k] = v
	}
	if now == nil {
		now = time.Now
	}
	return &Limiter{
		now:     now,
		def:     def,
		custom:  cp,
		buckets: make(map[string]*entry),
	}, nil
}

// limitFor returns the configured limit for a client key. Config map keys are
// raw API keys or IPs; the "key:" or "ip:" namespace prefix added by ClientKey
// is stripped before lookup.
func (l *Limiter) limitFor(key string) Limit {
	raw := key
	if rest, ok := strings.CutPrefix(key, "key:"); ok {
		raw = rest
	} else if rest, ok := strings.CutPrefix(key, "ip:"); ok {
		raw = rest
	}
	if c, ok := l.custom[raw]; ok {
		return c
	}
	return l.def
}

// Allow consumes a token for key. When denied, retryAfter is the number of
// whole seconds until a token is available (at least 1).
func (l *Limiter) Allow(key string) (allowed bool, retryAfter int) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.buckets[key]
	if !ok || e == nil {
		e = &entry{bucket: NewBucket(l.limitFor(key), now)}
		l.buckets[key] = e
	}
	e.lastSeen = now
	if e.bucket.Take(now) {
		return true, 0
	}
	return false, e.bucket.RetryAfter()
}

// Evict removes buckets idle for more than idle as of now. It returns the
// number of buckets removed.
func (l *Limiter) Evict(now time.Time, idle time.Duration) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for k, e := range l.buckets {
		if e == nil || now.Sub(e.lastSeen) > idle {
			delete(l.buckets, k)
			n++
		}
	}
	return n
}

// Len returns the number of tracked buckets.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// RunEvictor evicts idle buckets each time tick fires, until ctx is done or
// tick is closed.
func (l *Limiter) RunEvictor(ctx context.Context, tick <-chan time.Time, idle time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-tick:
			if !ok {
				return
			}
			l.Evict(l.now(), idle)
		}
	}
}

// ClientKey returns the X-Api-Key header, or the remote IP if it is missing.
func ClientKey(r *http.Request) string {
	if k := r.Header.Get("X-Api-Key"); k != "" {
		return "key:" + k
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// RemoteAddr without a port; use it as is.
		return "ip:" + r.RemoteAddr
	}
	return "ip:" + host
}

// Middleware rejects requests over the limit with 429 and Retry-After.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retry := l.Allow(ClientKey(r))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"msg": "hi"})
}

// NewHandler returns the application handler with the limiter applied.
func NewHandler(l *Limiter) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", helloHandler)
	return l.Middleware(mux)
}

// Serve runs the HTTP server on ln and the evictor until ctx is done, then
// shuts both down. The evictor is guaranteed to have stopped on return.
func Serve(ctx context.Context, ln net.Listener, l *Limiter) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	srv := &http.Server{
		Handler:           NewHandler(l),
		ReadHeaderTimeout: 5 * time.Second,
	}

	var wg sync.WaitGroup
	ticker := time.NewTicker(EvictInterval)
	defer ticker.Stop()
	wg.Go(func() {
		l.RunEvictor(ctx, ticker.C, IdleTimeout)
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errCh:
	}
	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	shutErr := srv.Shutdown(shutCtx)
	if serveErr == nil {
		serveErr = <-errCh
	}
	wg.Wait()

	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return shutErr
}
