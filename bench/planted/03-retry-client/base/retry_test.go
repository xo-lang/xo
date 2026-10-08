package benchtask

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock records requested sleeps and returns immediately.
type fakeClock struct {
	mu     sync.Mutex
	sleeps []time.Duration
	onSlp  func() // optional hook run on each Sleep before checking ctx
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	c.sleeps = append(c.sleeps, d)
	hook := c.onSlp
	c.mu.Unlock()
	if hook != nil {
		hook()
	}
	return ctx.Err()
}

func (c *fakeClock) Sleeps() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.sleeps)
}

// fixedRand always returns v.
type fixedRand float64

func (r fixedRand) Float64() float64 { return float64(r) }

// scripted returns a server that replies with the given handlers in order,
// repeating the last one, and counts calls.
func scripted(t *testing.T, steps ...func(w http.ResponseWriter)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(n.Add(1)) - 1
		if i >= len(steps) {
			i = len(steps) - 1
		}
		steps[i](w)
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

func status(code int) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) { w.WriteHeader(code) }
}

func statusWithRetryAfter(code int, ra string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Retry-After", ra)
		w.WriteHeader(code)
	}
}

func body(s string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(s))
	}
}

func asFetchErr(t *testing.T, err error) *FetchError {
	t.Helper()
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *FetchError, got %T: %v", err, err)
	}
	return fe
}

func TestSucceedsOnThirdAttemptAfterTwo503s(t *testing.T) {
	srv, calls := scripted(t, status(503), status(503), body("hello"))
	clk := &fakeClock{}

	got, err := GetWithRetry(context.Background(), srv.Client(), clk, fixedRand(0.5), srv.URL, DefaultRetryPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("body = %q, want hello", got)
	}
	if c := calls.Load(); c != 3 {
		t.Fatalf("calls = %d, want 3", c)
	}
	// rand 0.5 means zero jitter offset: 100ms then 200ms.
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	if s := clk.Sleeps(); !slices.Equal(s, want) {
		t.Fatalf("sleeps = %v, want %v", s, want)
	}
}

func TestJitterBounds(t *testing.T) {
	srv, _ := scripted(t, status(503), status(503), body("ok"))

	low := &fakeClock{}
	if _, err := GetWithRetry(context.Background(), srv.Client(), low, fixedRand(0), srv.URL, DefaultRetryPolicy()); err != nil {
		t.Fatal(err)
	}
	if want := []time.Duration{80 * time.Millisecond, 160 * time.Millisecond}; !slices.Equal(low.Sleeps(), want) {
		t.Fatalf("low jitter sleeps = %v, want %v", low.Sleeps(), want)
	}
}

func TestBackoffCappedByMaxDelay(t *testing.T) {
	p := DefaultRetryPolicy()
	cases := map[int]time.Duration{
		2:  100 * time.Millisecond,
		3:  200 * time.Millisecond,
		4:  400 * time.Millisecond,
		7:  3200 * time.Millisecond,
		8:  5 * time.Second,
		70: 5 * time.Second,
	}
	for n, want := range cases {
		if got := backoff(p, n); got != want {
			t.Errorf("backoff(%d) = %v, want %v", n, got, want)
		}
	}
}

func Test404ReturnsImmediately(t *testing.T) {
	srv, calls := scripted(t, status(404))
	clk := &fakeClock{}

	_, err := GetWithRetry(context.Background(), srv.Client(), clk, fixedRand(0.5), srv.URL, DefaultRetryPolicy())
	fe := asFetchErr(t, err)
	if fe.Kind != KindNonRetryableStatus {
		t.Fatalf("kind = %v, want non retryable status", fe.Kind)
	}
	if fe.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", fe.StatusCode)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error %q does not mention 404", err)
	}
	if c := calls.Load(); c != 1 {
		t.Fatalf("calls = %d, want 1", c)
	}
	if s := clk.Sleeps(); len(s) != 0 {
		t.Fatalf("unexpected sleeps %v", s)
	}
}

func TestRetryAfterRespected(t *testing.T) {
	srv, calls := scripted(t, statusWithRetryAfter(429, "2"), statusWithRetryAfter(503, "30"), body("done"))
	clk := &fakeClock{}

	got, err := GetWithRetry(context.Background(), srv.Client(), clk, fixedRand(0.9), srv.URL, DefaultRetryPolicy())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "done" || calls.Load() != 3 {
		t.Fatalf("got %q after %d calls", got, calls.Load())
	}
	// 2s from the header, then 30s capped to max_delay of 5s.
	want := []time.Duration{2 * time.Second, 5 * time.Second}
	if s := clk.Sleeps(); !slices.Equal(s, want) {
		t.Fatalf("sleeps = %v, want %v", s, want)
	}
}

func TestRetryAfterIgnoredOn500(t *testing.T) {
	srv, _ := scripted(t, statusWithRetryAfter(500, "3"), body("x"))
	clk := &fakeClock{}
	if _, err := GetWithRetry(context.Background(), srv.Client(), clk, fixedRand(0.5), srv.URL, DefaultRetryPolicy()); err != nil {
		t.Fatal(err)
	}
	if want := []time.Duration{100 * time.Millisecond}; !slices.Equal(clk.Sleeps(), want) {
		t.Fatalf("sleeps = %v, want %v", clk.Sleeps(), want)
	}
}

func TestExhaustedContainsLastStatus(t *testing.T) {
	srv, calls := scripted(t, status(500), status(502), status(504))
	clk := &fakeClock{}
	p := DefaultRetryPolicy()
	p.MaxAttempts = 3

	_, err := GetWithRetry(context.Background(), srv.Client(), clk, fixedRand(0.5), srv.URL, p)
	fe := asFetchErr(t, err)
	if fe.Kind != KindAttemptsExhausted {
		t.Fatalf("kind = %v, want attempts exhausted", fe.Kind)
	}
	if fe.Attempts != 3 || calls.Load() != 3 {
		t.Fatalf("attempts = %d, calls = %d, want 3", fe.Attempts, calls.Load())
	}
	if fe.StatusCode != 504 {
		t.Fatalf("status = %d, want 504", fe.StatusCode)
	}
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != 504 {
		t.Fatalf("expected wrapped StatusError 504, got %v", fe.Err)
	}
	if !strings.Contains(err.Error(), "504") {
		t.Fatalf("error %q does not mention 504", err)
	}
	if want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}; !slices.Equal(clk.Sleeps(), want) {
		t.Fatalf("sleeps = %v, want %v", clk.Sleeps(), want)
	}
}

func TestConnectionErrorsAreRetriedThenExhausted(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	u := srv.URL
	client := srv.Client()
	srv.Close() // nothing listening anymore: connection refused

	clk := &fakeClock{}
	p := DefaultRetryPolicy()
	p.MaxAttempts = 2
	_, err := GetWithRetry(context.Background(), client, clk, fixedRand(0.5), u, p)
	fe := asFetchErr(t, err)
	if fe.Kind != KindAttemptsExhausted || fe.Err == nil || fe.StatusCode != 0 {
		t.Fatalf("got %+v", fe)
	}
	if len(clk.Sleeps()) != 1 {
		t.Fatalf("sleeps = %v, want 1 sleep", clk.Sleeps())
	}
}

func TestInvalidURL(t *testing.T) {
	for _, u := range []string{"", "://bad", "ftp://example.com/x", "http://"} {
		_, err := GetWithRetry(context.Background(), http.DefaultClient, &fakeClock{}, fixedRand(0.5), u, DefaultRetryPolicy())
		fe := asFetchErr(t, err)
		if fe.Kind != KindInvalidURL {
			t.Errorf("%q: kind = %v, want invalid url", u, fe.Kind)
		}
	}
}

func TestCancelledDuringBackoff(t *testing.T) {
	srv, calls := scripted(t, status(503))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clk := &fakeClock{onSlp: cancel}

	_, err := GetWithRetry(ctx, srv.Client(), clk, fixedRand(0.5), srv.URL, DefaultRetryPolicy())
	fe := asFetchErr(t, err)
	if fe.Kind != KindCancelled {
		t.Fatalf("kind = %v, want cancelled", fe.Kind)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected errors.Is context.Canceled, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

func TestRealClockHonoursContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (RealClock{}).Sleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if err := (RealClock{}).Sleep(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func newFetcher(srv *httptest.Server, clk Clock) *Fetcher {
	return &Fetcher{Net: srv.Client(), Clock: clk, Rand: fixedRand(0.5), Policy: DefaultRetryPolicy(), BaseURL: srv.URL}
}

func TestFetchProfileSuccess(t *testing.T) {
	var path atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path.Store(r.URL.Path)
		_, _ = w.Write([]byte(`{"id":42,"name":"Ada","email":"ada@example.com"}`))
	}))
	defer srv.Close()

	p, err := newFetcher(srv, &fakeClock{}).FetchProfile(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if p != (Profile{ID: 42, Name: "Ada", Email: "ada@example.com"}) {
		t.Fatalf("profile = %+v", p)
	}
	if got := path.Load(); got != "/users/42" {
		t.Fatalf("path = %v", got)
	}
}

func TestFetchProfileDecodeError(t *testing.T) {
	srv, _ := scripted(t, body(`{"id": 1, "name": `))

	_, err := newFetcher(srv, &fakeClock{}).FetchProfile(context.Background(), 1)
	var pe *ProfileError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *ProfileError, got %T: %v", err, err)
	}
	if pe.Kind != ProfileKindDecode {
		t.Fatalf("kind = %v, want decode", pe.Kind)
	}
	var fe *FetchError
	if errors.As(err, &fe) {
		t.Fatalf("decode error should not wrap a FetchError")
	}
	var syn *json.SyntaxError
	var ute *json.UnmarshalTypeError
	if !errors.As(err, &syn) && !errors.As(err, &ute) && !strings.Contains(err.Error(), "unexpected end") {
		t.Fatalf("expected a JSON error, got %v", pe.Err)
	}
}

func TestFetchProfileWrapsFetchError(t *testing.T) {
	srv, _ := scripted(t, status(404))

	_, err := newFetcher(srv, &fakeClock{}).FetchProfile(context.Background(), 7)
	var pe *ProfileError
	if !errors.As(err, &pe) || pe.Kind != ProfileKindFetch {
		t.Fatalf("expected fetch ProfileError, got %v", err)
	}
	fe := asFetchErr(t, err)
	if fe.Kind != KindNonRetryableStatus || fe.StatusCode != 404 {
		t.Fatalf("inner = %+v", fe)
	}
}
