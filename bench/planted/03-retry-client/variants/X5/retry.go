// Package benchtask implements an HTTP GET helper with retries and a small
// error model, plus a higher level profile fetcher built on top of it.
package benchtask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Doer performs HTTP requests. *http.Client satisfies it.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Clock abstracts waiting so tests can inject a fake.
type Clock interface {
	// Sleep waits for d or until ctx is done, returning ctx.Err() in the
	// latter case.
	Sleep(ctx context.Context, d time.Duration) error
}

// Rand supplies uniform random numbers in [0, 1). *math/rand/v2.Rand
// satisfies it.
type Rand interface {
	Float64() float64
}

// RealClock waits using real timers.
type RealClock struct{}

// Sleep implements Clock.
func (RealClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// RetryPolicy controls the retry behavior of GetWithRetry.
type RetryPolicy struct {
	MaxAttempts int           // total attempts including the first; values < 1 mean 1
	BaseDelay   time.Duration // delay before attempt 2
	MaxDelay    time.Duration // cap on the un-jittered delay and on Retry-After
	Jitter      float64       // fraction in [0, 1]; delay varies by +/- Jitter*delay
}

// DefaultRetryPolicy returns the documented defaults.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 5,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Jitter:      0.2,
	}
}

// FetchErrKind classifies a FetchError.
type FetchErrKind int

// FetchError kinds.
const (
	KindInvalidURL FetchErrKind = iota + 1
	KindNonRetryableStatus
	KindAttemptsExhausted
	KindCancelled
)

func (k FetchErrKind) String() string {
	switch k {
	case KindInvalidURL:
		return "invalid url"
	case KindNonRetryableStatus:
		return "non retryable status"
	case KindAttemptsExhausted:
		return "attempts exhausted"
	case KindCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// FetchError is the error returned by GetWithRetry.
type FetchError struct {
	Kind       FetchErrKind
	URL        string
	Attempts   int   // attempts made
	StatusCode int   // status of the last response, 0 if the last attempt had no response
	Err        error // underlying (last) error, may be nil
}

func (e *FetchError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "fetch %q: %s", e.URL, e.Kind)
	switch e.Kind {
	case KindNonRetryableStatus:
		fmt.Fprintf(&b, ": status %d", e.StatusCode)
	case KindAttemptsExhausted:
		fmt.Fprintf(&b, " after %d attempts", e.Attempts)
	}
	if e.Err != nil {
		fmt.Fprintf(&b, ": %v", e.Err)
	}
	return b.String()
}

// Unwrap returns the underlying error.
func (e *FetchError) Unwrap() error { return e.Err }

// StatusError describes an HTTP response with an unwanted status code.
type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("status %d %s", e.StatusCode, http.StatusText(e.StatusCode))
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// backoff returns the un-jittered delay before attempt n (n >= 2):
// min(maxDelay, base * 2^(n-2)).
func backoff(p RetryPolicy, n int) time.Duration {
	d := p.BaseDelay
	if d <= 0 {
		return 0
	}
	for i := 2; i < n; i++ {
		if p.MaxDelay > 0 && d >= p.MaxDelay {
			break
		}
		if d > (1<<62)/2 {
			break // avoid overflow
		}
		d *= 2
	}
	if p.MaxDelay > 0 && d > p.MaxDelay {
		d = p.MaxDelay
	}
	return d
}

// jittered applies +/- jitter*d using r in [0, 1).
func jittered(d time.Duration, jitter float64, rnd Rand) time.Duration {
	if jitter <= 0 || d <= 0 || rnd == nil {
		return d
	}
	if jitter > 1 {
		jitter = 1
	}
	off := (2*rnd.Float64() - 1) * jitter * float64(d)
	out := time.Duration(math.Round(float64(d) + off))
	if out < 0 {
		out = 0
	}
	return out
}

// retryAfter parses a Retry-After header given in whole seconds.
func retryAfter(h string) (time.Duration, bool) {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0, false
	}
	secs, err := strconv.Atoi(h)
	if err != nil || secs < 0 {
		return 0, false
	}
	return time.Duration(secs) * time.Second, true
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("missing host")
	}
	return nil
}

// GetWithRetry performs an HTTP GET on rawURL, retrying transient failures
// according to policy. It returns the response body of the first 2xx
// response.
func GetWithRetry(ctx context.Context, net Doer, clock Clock, rnd Rand, rawURL string, policy RetryPolicy) ([]byte, error) {
	if err := validateURL(rawURL); err != nil {
		return nil, &FetchError{Kind: KindInvalidURL, URL: rawURL, Err: err}
	}
	maxAttempts := policy.MaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	cancelled := func(attempts, status int) error {
		return &FetchError{Kind: KindCancelled, URL: rawURL, Attempts: attempts, StatusCode: status, Err: ctx.Err()}
	}

	var (
		lastErr    error
		lastStatus int
		wait       time.Duration // delay override from Retry-After, -1 if none
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			d := wait
			if d < 0 {
				d = jittered(backoff(policy, attempt), policy.Jitter, rnd)
			}
			if err := clock.Sleep(ctx, d); err != nil {
				return nil, cancelled(attempt-1, lastStatus)
			}
		}
		if ctx.Err() != nil {
			return nil, cancelled(attempt-1, lastStatus)
		}
		wait = -1

		type result struct {
			body   []byte
			status int
			hdr    http.Header
			err    error
		}
		ch := make(chan result)
		go func() {
			b, s, h, e := doOnce(ctx, net, rawURL)
			ch <- result{b, s, h, e}
		}()
		var r result
		select {
		case <-ctx.Done():
			return nil, cancelled(attempt, 0)
		case r = <-ch:
		}
		body, status, hdr, err := r.body, r.status, r.hdr, r.err
		if err != nil {
			if ctx.Err() != nil {
				return nil, cancelled(attempt, 0)
			}
			// Connection errors, timeouts and body read failures are retryable.
			lastErr, lastStatus = err, status
			continue
		}
		if status >= 200 && status < 300 {
			return body, nil
		}
		serr := &StatusError{StatusCode: status}
		if !isRetryableStatus(status) {
			return nil, &FetchError{Kind: KindNonRetryableStatus, URL: rawURL, Attempts: attempt, StatusCode: status, Err: serr}
		}
		lastErr, lastStatus = serr, status
		if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			if d, ok := retryAfter(hdr.Get("Retry-After")); ok {
				if policy.MaxDelay > 0 && d > policy.MaxDelay {
					d = policy.MaxDelay
				}
				wait = d
			}
		}
	}
	return nil, &FetchError{Kind: KindAttemptsExhausted, URL: rawURL, Attempts: maxAttempts, StatusCode: lastStatus, Err: lastErr}
}

// doOnce performs a single GET. On a 2xx it returns the full body. For other
// statuses the body is drained and discarded. A non-nil error means a
// transport level failure.
func doOnce(ctx context.Context, net Doer, rawURL string) ([]byte, int, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, nil, err
	}
	resp, err := net.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, resp.StatusCode, resp.Header, fmt.Errorf("reading body: %w", err)
		}
		return body, resp.StatusCode, resp.Header, nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return nil, resp.StatusCode, resp.Header, nil
}
