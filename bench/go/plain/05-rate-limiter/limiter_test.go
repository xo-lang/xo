package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func doHello(t *testing.T, h http.Handler, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	if key != "" {
		req.Header.Set("X-Api-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTwentyThenLimited(t *testing.T) {
	clk := newFakeClock()
	h := NewHandler(NewLimiter(nil, WithClock(clk.Now)))
	for i := 1; i <= 20; i++ {
		rec := doHello(t, h, "k")
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want 200", i, rec.Code)
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["msg"] != "hi" {
			t.Fatalf("request %d: bad body %q (%v)", i, rec.Body.String(), err)
		}
	}
	rec := doHello(t, h, "k")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("21st: got %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("21st: missing Retry-After")
	}
}

func TestRefillAfterOneSecond(t *testing.T) {
	clk := newFakeClock()
	h := NewHandler(NewLimiter(nil, WithClock(clk.Now)))
	for range 20 {
		doHello(t, h, "k")
	}
	if rec := doHello(t, h, "k"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	clk.Advance(time.Second)
	for i := 1; i <= 5; i++ {
		if rec := doHello(t, h, "k"); rec.Code != http.StatusOK {
			t.Fatalf("refilled request %d: got %d, want 200", i, rec.Code)
		}
	}
	if rec := doHello(t, h, "k"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6th after refill: got %d, want 429", rec.Code)
	}
}

func TestRetryAfterValue(t *testing.T) {
	tests := []struct {
		name    string
		limit   Limit
		advance time.Duration
		want    string
	}{
		// 0 tokens, 5/s: 0.2s rounds up to 1.
		{"default rounds up to minimum 1", Limit{20, 5}, 0, "1"},
		// 0 tokens, 0.25/s: exactly 4s.
		{"slow refill exact", Limit{1, 0.25}, 0, "4"},
		// 0 tokens, 0.25/s, 1.5s later: 0.375 tokens, needs 2.5s, rounds to 3.
		{"slow refill partial", Limit{1, 0.25}, 1500 * time.Millisecond, "3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clk := newFakeClock()
			h := NewHandler(NewLimiter(map[string]Limit{"k": tc.limit}, WithClock(clk.Now)))
			for i := 0; i < int(tc.limit.Capacity); i++ {
				if rec := doHello(t, h, "k"); rec.Code != http.StatusOK {
					t.Fatalf("drain %d: got %d", i, rec.Code)
				}
			}
			clk.Advance(tc.advance)
			rec := doHello(t, h, "k")
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("got %d, want 429", rec.Code)
			}
			if got := rec.Header().Get("Retry-After"); got != tc.want {
				t.Fatalf("Retry-After = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRetryAfterIsHonest(t *testing.T) {
	// Waiting exactly Retry-After seconds must make the next request succeed.
	clk := newFakeClock()
	h := NewHandler(NewLimiter(map[string]Limit{"k": {1, 0.3}}, WithClock(clk.Now)))
	doHello(t, h, "k")
	rec := doHello(t, h, "k")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("got %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "4" {
		t.Fatalf("Retry-After = %q, want 4", got)
	}
	clk.Advance(4 * time.Second)
	if rec := doHello(t, h, "k"); rec.Code != http.StatusOK {
		t.Fatalf("after Retry-After: got %d, want 200", rec.Code)
	}
}

func TestCustomLimit(t *testing.T) {
	clk := newFakeClock()
	h := NewHandler(NewLimiter(map[string]Limit{"vip": {Capacity: 3, RefillPerSec: 1}}, WithClock(clk.Now)))
	for i := 1; i <= 3; i++ {
		if rec := doHello(t, h, "vip"); rec.Code != http.StatusOK {
			t.Fatalf("vip %d: got %d", i, rec.Code)
		}
	}
	if rec := doHello(t, h, "vip"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("vip 4th: got %d, want 429", rec.Code)
	}
	// Other keys still get the default of 20.
	for i := 1; i <= 20; i++ {
		if rec := doHello(t, h, "other"); rec.Code != http.StatusOK {
			t.Fatalf("other %d: got %d", i, rec.Code)
		}
	}
	clk.Advance(time.Second)
	if rec := doHello(t, h, "vip"); rec.Code != http.StatusOK {
		t.Fatalf("vip after 1s: got %d", rec.Code)
	}
	if rec := doHello(t, h, "vip"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("vip second after 1s: got %d, want 429", rec.Code)
	}
}

func TestRemoteIPFallback(t *testing.T) {
	clk := newFakeClock()
	h := NewHandler(NewLimiter(nil, WithClock(clk.Now)))
	send := func(addr string) int {
		req := httptest.NewRequest(http.MethodGet, "/hello", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// Different ports, same IP share a bucket.
	for i := range 20 {
		if c := send("10.0.0.1:" + string(rune('1'+i%9)) + "000"); c != http.StatusOK {
			t.Fatalf("req %d: %d", i, c)
		}
	}
	if c := send("10.0.0.1:5555"); c != http.StatusTooManyRequests {
		t.Fatalf("same IP: got %d, want 429", c)
	}
	if c := send("10.0.0.2:5555"); c != http.StatusOK {
		t.Fatalf("other IP: got %d, want 200", c)
	}
}

func TestIdleEviction(t *testing.T) {
	clk := newFakeClock()
	l := NewLimiter(nil, WithClock(clk.Now))
	l.Allow("old")
	clk.Advance(5 * time.Minute)
	l.Allow("recent")
	clk.Advance(5*time.Minute + time.Second) // old idle 10m1s, recent 5m1s
	if n := l.Evict(); n != 1 {
		t.Fatalf("evicted %d, want 1", n)
	}
	if l.Len() != 1 {
		t.Fatalf("len %d, want 1", l.Len())
	}
	// Exactly 10 minutes is not "more than" 10 minutes.
	clk.Advance(5*time.Minute - time.Second)
	if n := l.Evict(); n != 0 {
		t.Fatalf("evicted %d at exactly 10m, want 0", n)
	}
	clk.Advance(time.Nanosecond)
	if n := l.Evict(); n != 1 {
		t.Fatalf("evicted %d, want 1", n)
	}
	// An evicted key starts again with a full bucket.
	for i := range 20 {
		if ok, _ := l.Allow("old"); !ok {
			t.Fatalf("fresh bucket request %d denied", i)
		}
	}
}

func TestEvictorRunsAndStops(t *testing.T) {
	clk := newFakeClock()
	l := NewLimiter(nil, WithClock(clk.Now))
	l.Allow("a")
	clk.Advance(11 * time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	done := l.RunEvictor(ctx, 5*time.Millisecond)
	deadline := time.After(5 * time.Second)
	for l.Len() != 0 {
		select {
		case <-deadline:
			t.Fatal("evictor did not evict")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("evictor did not stop")
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- Run(ctx, "127.0.0.1:0", NewLimiter(nil)) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestConcurrentRequests(t *testing.T) {
	clk := newFakeClock()
	h := NewHandler(NewLimiter(nil, WithClock(clk.Now)))
	srv := httptest.NewServer(h)
	defer srv.Close()
	var ok, limited atomic.Int64
	var wg sync.WaitGroup
	for range 60 {
		wg.Go(func() {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+"/hello", nil)
			req.Header.Set("X-Api-Key", "shared")
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK:
				ok.Add(1)
			case http.StatusTooManyRequests:
				limited.Add(1)
			}
		})
	}
	wg.Wait()
	if ok.Load() != 20 || limited.Load() != 40 {
		t.Fatalf("ok=%d limited=%d, want 20/40", ok.Load(), limited.Load())
	}
}

func TestBucketFractional(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewBucket(Limit{Capacity: 2, RefillPerSec: 1}, now)
	b.Take(now)
	b.Take(now)
	now = now.Add(500 * time.Millisecond)
	if b.Take(now) {
		t.Fatal("took with 0.5 tokens")
	}
	now = now.Add(500 * time.Millisecond)
	if !b.Take(now) {
		t.Fatal("denied with 1.0 tokens")
	}
	now = now.Add(time.Hour)
	b.refill(now)
	if b.Tokens != 2 {
		t.Fatalf("tokens %v, want capped at 2", b.Tokens)
	}
}
