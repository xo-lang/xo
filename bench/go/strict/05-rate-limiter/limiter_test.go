package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
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

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

var defLimit = Limit{Capacity: DefaultCapacity, RefillPerSec: DefaultRefillPerSec}

func newTestLimiter(t *testing.T, custom map[string]Limit) (*Limiter, *fakeClock) {
	t.Helper()
	clk := newFakeClock()
	l, err := NewLimiter(defLimit, custom, clk.Now)
	if err != nil {
		t.Fatalf("NewLimiter: %v", err)
	}
	return l, clk
}

func doReq(h http.Handler, apiKey, remote string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	if apiKey != "" {
		req.Header.Set("X-Api-Key", apiKey)
	}
	if remote != "" {
		req.RemoteAddr = remote
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestBurstThen429(t *testing.T) {
	l, _ := newTestLimiter(t, nil)
	h := NewHandler(l)
	for i := range 20 {
		rec := doReq(h, "abc", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i+1, rec.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body["msg"] != "hi" {
			t.Fatalf("body = %v", body)
		}
	}
	rec := doReq(h, "abc", "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("21st: status %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}

func TestRefillAfterOneSecond(t *testing.T) {
	l, clk := newTestLimiter(t, nil)
	h := NewHandler(l)
	for range 20 {
		doReq(h, "abc", "")
	}
	if rec := doReq(h, "abc", ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429", rec.Code)
	}
	clk.Advance(time.Second)
	for i := range 5 {
		if rec := doReq(h, "abc", ""); rec.Code != http.StatusOK {
			t.Fatalf("after refill request %d: status %d, want 200", i+1, rec.Code)
		}
	}
	if rec := doReq(h, "abc", ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6th after refill: status %d, want 429", rec.Code)
	}
}

func TestRetryAfter(t *testing.T) {
	custom := map[string]Limit{
		"slow": {Capacity: 1, RefillPerSec: 0.5},
		"tiny": {Capacity: 1, RefillPerSec: 0.1},
	}
	l, clk := newTestLimiter(t, custom)
	h := NewHandler(l)

	// Default: 0 tokens, 5/sec -> 0.2s -> rounded up to 1.
	for range 20 {
		doReq(h, "d", "")
	}
	if got := doReq(h, "d", "").Header().Get("Retry-After"); got != "1" {
		t.Errorf("default Retry-After = %q, want 1", got)
	}

	// 0.5/sec: one token needs 2s.
	doReq(h, "slow", "")
	if got := doReq(h, "slow", "").Header().Get("Retry-After"); got != "2" {
		t.Errorf("slow Retry-After = %q, want 2", got)
	}
	// After 0.5s, tokens = 0.25, need 0.75 -> 1.5s -> 2.
	clk.Advance(500 * time.Millisecond)
	if got := doReq(h, "slow", "").Header().Get("Retry-After"); got != "2" {
		t.Errorf("slow after 0.5s Retry-After = %q, want 2", got)
	}
	// After another 1s, tokens = 0.75, need 0.25 -> 0.5s -> 1.
	clk.Advance(time.Second)
	if got := doReq(h, "slow", "").Header().Get("Retry-After"); got != "1" {
		t.Errorf("slow after 1.5s Retry-After = %q, want 1", got)
	}

	// 0.1/sec: 10s.
	doReq(h, "tiny", "")
	if got := doReq(h, "tiny", "").Header().Get("Retry-After"); got != "10" {
		t.Errorf("tiny Retry-After = %q, want 10", got)
	}
}

func TestCustomLimit(t *testing.T) {
	l, clk := newTestLimiter(t, map[string]Limit{"vip": {Capacity: 3, RefillPerSec: 1}})
	h := NewHandler(l)
	for i := range 3 {
		if rec := doReq(h, "vip", ""); rec.Code != http.StatusOK {
			t.Fatalf("vip %d: status %d", i+1, rec.Code)
		}
	}
	if rec := doReq(h, "vip", ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("vip 4th: status %d, want 429", rec.Code)
	}
	clk.Advance(time.Second)
	if rec := doReq(h, "vip", ""); rec.Code != http.StatusOK {
		t.Fatalf("vip after 1s: status %d, want 200", rec.Code)
	}
	if rec := doReq(h, "vip", ""); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("vip second after 1s: status %d, want 429", rec.Code)
	}
	// Other keys keep the default.
	for i := range 20 {
		if rec := doReq(h, "other", ""); rec.Code != http.StatusOK {
			t.Fatalf("other %d: status %d", i+1, rec.Code)
		}
	}
}

func TestKeyFallsBackToRemoteIP(t *testing.T) {
	// Same IP, different ports share a bucket.
	l2, _ := newTestLimiter(t, nil)
	h2 := NewHandler(l2)
	for i := range 20 {
		port := 1000 + i
		if rec := doReq(h2, "", net.JoinHostPort("10.0.0.1", strconv.Itoa(port))); rec.Code != http.StatusOK {
			t.Fatalf("ip req %d: %d", i+1, rec.Code)
		}
	}
	if rec := doReq(h2, "", "10.0.0.1:9999"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("ip 21st: %d, want 429", rec.Code)
	}
	if rec := doReq(h2, "", "10.0.0.2:9999"); rec.Code != http.StatusOK {
		t.Fatalf("other ip: %d, want 200", rec.Code)
	}
	// An API key from the exhausted IP gets its own bucket.
	if rec := doReq(h2, "k", "10.0.0.1:9999"); rec.Code != http.StatusOK {
		t.Fatalf("api key from same ip: %d, want 200", rec.Code)
	}
}

func TestIdleBucketsEvicted(t *testing.T) {
	l, clk := newTestLimiter(t, nil)
	l.Allow("a")
	clk.Advance(5 * time.Minute)
	l.Allow("b")
	if l.Len() != 2 {
		t.Fatalf("len = %d, want 2", l.Len())
	}
	clk.Advance(5*time.Minute + time.Second) // a idle 10m1s, b idle 5m1s
	if n := l.Evict(clk.Now(), IdleTimeout); n != 1 {
		t.Fatalf("evicted %d, want 1", n)
	}
	if l.Len() != 1 {
		t.Fatalf("len = %d, want 1", l.Len())
	}

	// Run the background evictor with a manual tick and stop it via ctx.
	tick := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		l.RunEvictor(ctx, tick, IdleTimeout)
		close(done)
	}()
	clk.Advance(6 * time.Minute) // b idle 11m1s
	tick <- clk.Now()
	// The send returns once the evictor received the tick; a second send
	// guarantees the first eviction completed.
	tick <- clk.Now()
	if l.Len() != 0 {
		t.Fatalf("len after evictor = %d, want 0", l.Len())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("evictor did not stop after cancel")
	}

	// A new request after eviction gets a fresh full bucket.
	for i := range 20 {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("fresh bucket request %d denied", i+1)
		}
	}
}

func TestNewLimiterValidates(t *testing.T) {
	if _, err := NewLimiter(Limit{Capacity: 0, RefillPerSec: 1}, nil, nil); err == nil {
		t.Error("expected error for zero capacity")
	}
	if _, err := NewLimiter(defLimit, map[string]Limit{"x": {Capacity: 5, RefillPerSec: 0}}, nil); err == nil {
		t.Error("expected error for zero refill")
	}
}

func TestConcurrentAllow(t *testing.T) {
	l, _ := newTestLimiter(t, nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for range 100 {
		wg.Go(func() {
			if ok, _ := l.Allow("c"); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if allowed != 20 {
		t.Fatalf("allowed = %d, want 20", allowed)
	}
}

func TestServeStops(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	l, _ := newTestLimiter(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- Serve(ctx, ln, l) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/hello")
	if err != nil || resp == nil {
		cancel()
		t.Fatalf("get: resp %v err %v", resp, err)
	}
	body, err := io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); cerr != nil {
		t.Errorf("close body: %v", cerr)
	}
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var m map[string]string
	if err := json.Unmarshal(body, &m); err != nil || m["msg"] != "hi" {
		t.Fatalf("body %q err %v", body, err)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop")
	}
}
