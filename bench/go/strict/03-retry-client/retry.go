// Package benchtask implements an HTTP GET helper with retries, exponential
// backoff with jitter, Retry-After support, and a small error model.
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

// Clock waits between attempts. Sleep must return a non nil error (normally
// ctx.Err()) if the context is done before or during the wait.
type Clock interface {
	Sleep(ctx context.Context, d time.Duration) error
}

// Rand supplies uniformly distributed values in [0, 1). *math/rand/v2.Rand
// satisfies it.
type Rand interface {
	Float64() float64
}

// RealClock sleeps using real timers.
type RealClock struct{}

// Sleep waits for d or until ctx is done, whichever comes first.
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

// RetryPolicy controls how many attempts are made and how long to wait
// between them.
type RetryPolicy struct {
	MaxAttempts int           // total attempts including the first, default 5
	BaseDelay   time.Duration // delay before attempt 2, default 100ms
	MaxDelay    time.Duration // cap on any single delay, default 5s
	Jitter      float64       // fraction of the delay to randomize by, default 0.2
}

// DefaultRetryPolicy returns the policy with the documented defaults.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 5,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    5 * time.Second,
		Jitter:      0.2,
	}
}

// normalized clamps out of range values to safe ones.
func (p RetryPolicy) normalized() RetryPolicy {
	if p.MaxAttempts < 1 {
		p.MaxAttempts = 1
	}
	if p.BaseDelay < 0 {
		p.BaseDelay = 0
	}
	if p.MaxDelay < 0 {
		p.MaxDelay = 0
	}
	if math.IsNaN(p.Jitter) || p.Jitter < 0 {
		p.Jitter = 0
	}
	if p.Jitter > 1 {
		p.Jitter = 1
	}
	return p
}

// backoff returns the delay before attempt n (n >= 2), given a random value r
// in [0, 1]. The base is min(MaxDelay, BaseDelay*2^(n-2)); jitter then moves
// it by up to plus or minus Jitter of that base.
func (p RetryPolicy) backoff(n int, r float64) time.Duration {
	d := p.BaseDelay
	for i := 2; i < n && d < p.MaxDelay; i++ {
		if d > math.MaxInt64/2 {
			d = p.MaxDelay
			break
		}
		d *= 2
	}
	if d > p.MaxDelay {
		d = p.MaxDelay
	}
	if p.Jitter > 0 && d > 0 {
		if math.IsNaN(r) || r < 0 {
			r = 0
		}
		if r > 1 {
			r = 1
		}
		delta := float64(d) * p.Jitter * (2*r - 1)
		j := float64(d) + delta
		if j >= math.MaxInt64 {
			return time.Duration(math.MaxInt64)
		}
		d = time.Duration(j)
	}
	if d < 0 {
		d = 0
	}
	return d
}

// FetchErrKind classifies a FetchError.
type FetchErrKind int

// FetchErrKind values.
const (
	KindInvalidURL FetchErrKind = iota + 1
	KindNonRetryableStatus
	KindAttemptsExhausted
	KindCancelled
)

// String returns a short name for the kind.
func (k FetchErrKind) String() string {
	switch k {
	case KindInvalidURL:
		return "invalid URL"
	case KindNonRetryableStatus:
		return "non-retryable status"
	case KindAttemptsExhausted:
		return "attempts exhausted"
	case KindCancelled:
		return "cancelled"
	}
	return "FetchErrKind(" + strconv.Itoa(int(k)) + ")"
}

// FetchError is the error returned by GetWithRetry.
type FetchError struct {
	Kind     FetchErrKind
	URL      string // redacted URL, or the raw input for KindInvalidURL
	Status   int    // HTTP status of the last response, 0 if none
	Attempts int    // number of requests actually sent
	Err      error  // underlying cause (last error for KindAttemptsExhausted)
}

func (e *FetchError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "fetch %q: ", e.URL)
	switch e.Kind {
	case KindInvalidURL:
		b.WriteString("invalid URL")
	case KindNonRetryableStatus:
		fmt.Fprintf(&b, "non-retryable status %d", e.Status)
	case KindAttemptsExhausted:
		fmt.Fprintf(&b, "attempts exhausted after %d attempts", e.Attempts)
		if e.Status != 0 {
			fmt.Fprintf(&b, ", last status %d", e.Status)
		}
	case KindCancelled:
		fmt.Fprintf(&b, "cancelled after %d attempts", e.Attempts)
	default:
		b.WriteString(e.Kind.String())
	}
	if e.Err != nil {
		b.WriteString(": ")
		b.WriteString(e.Err.Error())
	}
	return b.String()
}

// Unwrap returns the underlying cause.
func (e *FetchError) Unwrap() error { return e.Err }

// StatusError describes an unsuccessful HTTP response.
type StatusError struct {
	Code int
	Body string // possibly truncated response body, for diagnostics
}

func (e *StatusError) Error() string {
	msg := fmt.Sprintf("unexpected status %d %s", e.Code, http.StatusText(e.Code))
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

const (
	maxBodyBytes    = 32 << 20
	maxSnippetBytes = 512
)

var (
	errBodyTooLarge = errors.New("response body too large")
	errNilResponse  = errors.New("doer returned nil response and nil error")
)

// GetWithRetry performs a GET of rawURL, retrying connection errors, timeouts
// and statuses 429, 500, 502, 503 and 504 according to policy. It returns the
// body of the first 2xx response. Errors are always *FetchError.
func GetWithRetry(ctx context.Context, doer Doer, clock Clock, rnd Rand, rawURL string, policy RetryPolicy) ([]byte, error) {
	p := policy.normalized()

	u, err := parseURL(rawURL)
	if err != nil {
		return nil, &FetchError{Kind: KindInvalidURL, URL: rawURL, Err: err}
	}
	display := u.Redacted()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &FetchError{Kind: KindInvalidURL, URL: display, Err: err}
	}

	var (
		lastErr    error
		lastStatus int
		wait       time.Duration
	)
	cancelled := func(attempts int, cause error) error {
		return &FetchError{Kind: KindCancelled, URL: display, Status: lastStatus, Attempts: attempts, Err: cause}
	}

	for attempt := 1; attempt <= p.MaxAttempts; attempt++ {
		if attempt > 1 {
			if err := clock.Sleep(ctx, wait); err != nil {
				return nil, cancelled(attempt-1, err)
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, cancelled(attempt-1, err)
		}

		resp, err := doer.Do(req.Clone(ctx))
		if err == nil && resp == nil {
			// A misbehaving Doer; treat it like a transport failure.
			err = errNilResponse
		}
		if err != nil || resp == nil {
			if cerr := ctx.Err(); cerr != nil {
				return nil, cancelled(attempt, cerr)
			}
			lastErr, lastStatus = err, 0
			wait = p.backoff(attempt+1, rnd.Float64())
			continue
		}

		code := resp.StatusCode
		if code >= 200 && code <= 299 {
			body, err := readAndClose(resp.Body, maxBodyBytes, false)
			if err == nil {
				return body, nil
			}
			if cerr := ctx.Err(); cerr != nil {
				return nil, cancelled(attempt, cerr)
			}
			lastErr, lastStatus = fmt.Errorf("reading body: %w", err), code
			wait = p.backoff(attempt+1, rnd.Float64())
			continue
		}

		retryAfterHeader := resp.Header.Get("Retry-After")
		snippet, err := readAndClose(resp.Body, maxSnippetBytes, true)
		if err != nil {
			// The body is only diagnostic for error responses.
			snippet = nil
		}
		serr := &StatusError{Code: code, Body: strings.TrimSpace(string(snippet))}
		if !retryableStatus(code) {
			return nil, &FetchError{Kind: KindNonRetryableStatus, URL: display, Status: code, Attempts: attempt, Err: serr}
		}
		lastErr, lastStatus = serr, code
		if ra, ok := parseRetryAfter(retryAfterHeader, p.MaxDelay); ok && (code == http.StatusTooManyRequests || code == http.StatusServiceUnavailable) {
			wait = ra
		} else {
			wait = p.backoff(attempt+1, rnd.Float64())
		}
	}

	return nil, &FetchError{Kind: KindAttemptsExhausted, URL: display, Status: lastStatus, Attempts: p.MaxAttempts, Err: lastErr}
}

func parseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("missing host")
	}
	return u, nil
}

func retryableStatus(code int) bool {
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

// parseRetryAfter parses a Retry-After value given in whole seconds and caps
// it at maxDelay. HTTP-date values are not supported and report false.
func parseRetryAfter(v string, maxDelay time.Duration) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	secs, err := strconv.ParseInt(v, 10, 64)
	if err != nil || secs < 0 {
		return 0, false
	}
	if secs > int64(maxDelay/time.Second) {
		return maxDelay, true
	}
	d := time.Duration(secs) * time.Second
	return min(d, maxDelay), true
}

// readAndClose reads at most limit bytes and always closes rc. If the body is
// longer than limit it is truncated when truncate is set, otherwise an error
// is returned.
func readAndClose(rc io.ReadCloser, limit int64, truncate bool) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	cerr := rc.Close()
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		if truncate {
			return data[:limit], nil
		}
		return nil, errBodyTooLarge
	}
	if cerr != nil {
		return nil, cerr
	}
	return data, nil
}
