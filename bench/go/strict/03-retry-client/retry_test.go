package benchtask

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock records requested sleeps without waiting.
type fakeClock struct {
	sleeps  []time.Duration
	onSleep func(n int)
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	if c.onSleep != nil {
		c.onSleep(len(c.sleeps))
	}
	return ctx.Err()
}

// fixedRand always returns the same value. 0.5 means zero jitter.
type fixedRand float64

func (r fixedRand) Float64() float64 { return float64(r) }

type step struct {
	status int
	header map[string]string
	body   string
}

// newSeqServer serves the given steps in order, repeating the last one.
func newSeqServer(t *testing.T, steps ...step) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(hits.Add(1))
		s := steps[min(n, len(steps))-1]
		for k, v := range s.header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(s.status)
		if _, err := io.WriteString(w, s.body); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func testPolicy() RetryPolicy {
	return DefaultRetryPolicy()
}

func asFetchError(t *testing.T, err error) *FetchError {
	t.Helper()
	var fe *FetchError
	if !errors.As(err, &fe) || fe == nil {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	return fe
}

func TestSucceedsOnThirdAttemptAfterTwo503s(t *testing.T) {
	srv, hits := newSeqServer(t,
		step{status: http.StatusServiceUnavailable},
		step{status: http.StatusServiceUnavailable},
		step{status: http.StatusOK, body: "hello"},
	)
	clock := &fakeClock{}
	body, err := GetWithRetry(t.Context(), srv.Client(), clock, fixedRand(0.5), srv.URL, testPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "hello" {
		t.Errorf("body = %q, want %q", body, "hello")
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	if !slices.Equal(clock.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", clock.sleeps, want)
	}
}

func TestJitterBounds(t *testing.T) {
	p := DefaultRetryPolicy()
	for _, tc := range []struct {
		r        float64
		min, max time.Duration
	}{
		{0, 79 * time.Millisecond, 81 * time.Millisecond},
		{0.5, 100 * time.Millisecond, 100 * time.Millisecond},
		{0.999999, 119 * time.Millisecond, 121 * time.Millisecond},
	} {
		d := p.backoff(2, tc.r)
		if d < tc.min || d > tc.max {
			t.Errorf("backoff(2, %v) = %v, want in [%v, %v]", tc.r, d, tc.min, tc.max)
		}
	}
	// Jitter applies to the capped value, so it may move above MaxDelay.
	if d := p.backoff(20, 0); d < 3999*time.Millisecond || d > 4001*time.Millisecond {
		t.Errorf("capped backoff with r=0 = %v, want about 4s", d)
	}
}

func TestBackoffSchedule(t *testing.T) {
	p := DefaultRetryPolicy()
	p.Jitter = 0
	want := map[int]time.Duration{
		2:  100 * time.Millisecond,
		3:  200 * time.Millisecond,
		4:  400 * time.Millisecond,
		7:  3200 * time.Millisecond,
		8:  5 * time.Second,
		70: 5 * time.Second,
	}
	for n, w := range want {
		if got := p.backoff(n, 0.5); got != w {
			t.Errorf("backoff(%d) = %v, want %v", n, got, w)
		}
	}
}

func Test404ReturnsImmediately(t *testing.T) {
	srv, hits := newSeqServer(t, step{status: http.StatusNotFound, body: "no such thing"})
	clock := &fakeClock{}
	_, err := GetWithRetry(t.Context(), srv.Client(), clock, fixedRand(0.5), srv.URL, testPolicy())
	fe := asFetchError(t, err)
	if fe.Kind != KindNonRetryableStatus {
		t.Errorf("kind = %v, want %v", fe.Kind, KindNonRetryableStatus)
	}
	if fe.Status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", fe.Status)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error %q does not mention 404", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
	if len(clock.sleeps) != 0 {
		t.Errorf("sleeps = %v, want none", clock.sleeps)
	}
}

func TestRetryAfterRespected(t *testing.T) {
	srv, hits := newSeqServer(t,
		step{status: http.StatusTooManyRequests, header: map[string]string{"Retry-After": "2"}},
		step{status: http.StatusServiceUnavailable, header: map[string]string{"Retry-After": "30"}},
		step{status: http.StatusOK, body: "ok"},
	)
	clock := &fakeClock{}
	body, err := GetWithRetry(t.Context(), srv.Client(), clock, fixedRand(0.9), srv.URL, testPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q", body)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
	want := []time.Duration{2 * time.Second, 5 * time.Second} // second capped by MaxDelay
	if !slices.Equal(clock.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", clock.sleeps, want)
	}
}

func TestRetryAfterIgnoredFor500(t *testing.T) {
	srv, _ := newSeqServer(t,
		step{status: http.StatusInternalServerError, header: map[string]string{"Retry-After": "3"}},
		step{status: http.StatusOK},
	)
	clock := &fakeClock{}
	if _, err := GetWithRetry(t.Context(), srv.Client(), clock, fixedRand(0.5), srv.URL, testPolicy()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []time.Duration{100 * time.Millisecond}
	if !slices.Equal(clock.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", clock.sleeps, want)
	}
}

func TestExhaustedContainsLastStatus(t *testing.T) {
	srv, hits := newSeqServer(t,
		step{status: http.StatusServiceUnavailable},
		step{status: http.StatusBadGateway, body: "upstream down"},
	)
	clock := &fakeClock{}
	p := testPolicy()
	p.MaxAttempts = 3
	_, err := GetWithRetry(t.Context(), srv.Client(), clock, fixedRand(0.5), srv.URL, p)
	fe := asFetchError(t, err)
	if fe.Kind != KindAttemptsExhausted {
		t.Fatalf("kind = %v, want %v", fe.Kind, KindAttemptsExhausted)
	}
	if fe.Status != http.StatusBadGateway || fe.Attempts != 3 {
		t.Errorf("status = %d attempts = %d, want 502 and 3", fe.Status, fe.Attempts)
	}
	var se *StatusError
	if !errors.As(err, &se) || se == nil || se.Code != http.StatusBadGateway {
		t.Errorf("expected wrapped *StatusError with 502, got %v", err)
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "upstream down") {
		t.Errorf("error %q missing last status or body", err)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	if !slices.Equal(clock.sleeps, want) {
		t.Errorf("sleeps = %v, want %v", clock.sleeps, want)
	}
}

func TestConnectionErrorsAreRetried(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close() // nothing listens on addr now
	clock := &fakeClock{}
	p := testPolicy()
	p.MaxAttempts = 3
	_, err := GetWithRetry(t.Context(), &http.Client{Timeout: 5 * time.Second}, clock, fixedRand(0.5), addr, p)
	fe := asFetchError(t, err)
	if fe.Kind != KindAttemptsExhausted {
		t.Fatalf("kind = %v, want %v (%v)", fe.Kind, KindAttemptsExhausted, err)
	}
	if fe.Status != 0 || fe.Err == nil {
		t.Errorf("status = %d err = %v, want 0 and a transport error", fe.Status, fe.Err)
	}
	if len(clock.sleeps) != 2 {
		t.Errorf("sleeps = %v, want 2", clock.sleeps)
	}
}

func TestInvalidURL(t *testing.T) {
	for _, raw := range []string{"::not a url", "ftp://example.com/x", "http://", "/relative"} {
		clock := &fakeClock{}
		_, err := GetWithRetry(t.Context(), http.DefaultClient, clock, fixedRand(0.5), raw, testPolicy())
		fe := asFetchError(t, err)
		if fe.Kind != KindInvalidURL {
			t.Errorf("%q: kind = %v, want %v", raw, fe.Kind, KindInvalidURL)
		}
	}
}

func TestCancelledDuringBackoff(t *testing.T) {
	srv, hits := newSeqServer(t, step{status: http.StatusServiceUnavailable})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	clock := &fakeClock{onSleep: func(int) { cancel() }}
	_, err := GetWithRetry(ctx, srv.Client(), clock, fixedRand(0.5), srv.URL, testPolicy())
	fe := asFetchError(t, err)
	if fe.Kind != KindCancelled {
		t.Errorf("kind = %v, want %v", fe.Kind, KindCancelled)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error %v does not wrap context.Canceled", err)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1", got)
	}
}

func TestRealClockHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Now()
	if err := (RealClock{}).Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > time.Second {
		t.Errorf("sleep did not return promptly")
	}
	if err := (RealClock{}).Sleep(t.Context(), time.Millisecond); err != nil {
		t.Errorf("short sleep err = %v", err)
	}
}

func TestFetchProfileDecodeError(t *testing.T) {
	srv, _ := newSeqServer(t, step{status: http.StatusOK, body: "{not json"})
	_, err := FetchProfile(t.Context(), srv.Client(), &fakeClock{}, fixedRand(0.5), srv.URL, 42, testPolicy())
	var pe *ProfileError
	if !errors.As(err, &pe) || pe == nil {
		t.Fatalf("expected *ProfileError, got %T: %v", err, err)
	}
	if pe.Kind != ProfileErrDecode {
		t.Errorf("kind = %v, want decode", pe.Kind)
	}
	var se *json.SyntaxError
	if !errors.As(err, &se) {
		t.Errorf("expected wrapped *json.SyntaxError, got %v", err)
	}
	var fe *FetchError
	if errors.As(err, &fe) {
		t.Errorf("decode error should not wrap a FetchError")
	}
}

func TestFetchProfileSuccessAndPath(t *testing.T) {
	var gotPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath.Store(r.URL.Path)
		if _, err := io.WriteString(w, `{"id":7,"name":"Ada","email":"ada@example.com"}`); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	p, err := FetchProfile(t.Context(), srv.Client(), &fakeClock{}, fixedRand(0.5), srv.URL+"/", 7, testPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Profile{ID: 7, Name: "Ada", Email: "ada@example.com"}
	if p != want {
		t.Errorf("profile = %+v, want %+v", p, want)
	}
	if path, _ := gotPath.Load().(string); path != "/users/7" {
		t.Errorf("path = %q, want /users/7", path)
	}
}

func TestFetchProfileWrapsFetchError(t *testing.T) {
	srv, _ := newSeqServer(t, step{status: http.StatusForbidden})
	_, err := FetchProfile(t.Context(), srv.Client(), &fakeClock{}, fixedRand(0.5), srv.URL, 1, testPolicy())
	var pe *ProfileError
	if !errors.As(err, &pe) || pe == nil || pe.Kind != ProfileErrFetch {
		t.Fatalf("expected fetch ProfileError, got %v", err)
	}
	fe := asFetchError(t, err)
	if fe.Kind != KindNonRetryableStatus || fe.Status != http.StatusForbidden {
		t.Errorf("fetch error = %+v", fe)
	}
}
