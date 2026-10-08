# Task 03: Retry client

Write an HTTP GET helper with retries and a small module error model.

## Requirements

- `get_with_retry(net, clock, rand, url: Str, policy: RetryPolicy) -> Bytes ! FetchErr`
- `RetryPolicy`: `max_attempts` (default 5), `base_delay` (default 100ms),
  `max_delay` (default 5s), `jitter` (fraction, default 0.2).
- Delay before attempt n (n starting at 2) is
  `min(max_delay, base_delay * 2^(n-2))`, plus or minus up to `jitter` of that value.
- Retry on: connection errors, timeouts, 429, 500, 502, 503, 504.
  If a 429 or 503 response has a `Retry-After` header in seconds, wait that
  long instead (still capped by `max_delay`).
- Do not retry on other 4xx. Return immediately with an error that includes the
  status code.
- `FetchErr` must distinguish: invalid URL, non retryable status, attempts
  exhausted (including the last underlying error), and cancelled.
- A higher level function `fetch_profile(..., user_id: Int) -> Profile ! ProfileErr`
  calls `get_with_retry`, decodes JSON, and has its own error type that wraps
  `FetchErr` and adds a decode error.

## Tests to write

- Succeeds on the third attempt after two 503s; verify delays using a fake clock.
- 404 returns immediately after one attempt.
- Retry-After header is respected.
- Exhausted attempts error contains the last status.
- `fetch_profile` reports a decode error for invalid JSON.
