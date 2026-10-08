# Notes

## Design

- `GetWithRetry(ctx, net Doer, clock Clock, rnd Rand, rawURL string, policy RetryPolicy) ([]byte, error)`.
  `Doer` is satisfied by `*http.Client`, `Rand` by `*math/rand/v2.Rand`, `Clock` has a single
  `Sleep(ctx, d) error`. `RealClock` is the production clock; tests use a recording fake clock.
- `FetchError` is one struct with a `Kind` (invalid URL, non retryable status, attempts exhausted,
  cancelled), `StatusCode`, `Attempts`, and a wrapped `Err` (last underlying error). Status failures
  are wrapped as `*StatusError`, so `errors.As` works for both levels. Cancelled wraps `ctx.Err()`,
  so `errors.Is(err, context.Canceled)` holds.
- `Fetcher.FetchProfile(ctx, userID)` GETs `BaseURL/users/{id}` and returns `*ProfileError` with
  kind fetch (wrapping `*FetchError`) or decode (wrapping the `encoding/json` error).

## Ambiguities resolved

- Language neutral signature: added a `context.Context` as the first argument (idiomatic Go, and the
  natural source of "cancelled").
- Defaults: `DefaultRetryPolicy()` returns 5 / 100ms / 5s / 0.2. Zero fields are used as given rather
  than silently replaced, so `Jitter: 0` means no jitter. `MaxAttempts < 1` is treated as 1.
- Jitter is applied after the `max_delay` cap ("plus or minus up to jitter of that value"), so a
  jittered delay can exceed `max_delay` by up to `jitter`. Formula: `d + (2r - 1) * jitter * d`
  with r in [0, 1), rounded to the nearest nanosecond.
- Retry-After: only honored on 429 and 503, only the integer seconds form (HTTP-date ignored, falls
  back to normal backoff), capped by `max_delay`, and no jitter added on top.
- "Other 4xx are not retried": I also treat every non 2xx status outside the retry set (for example
  501, 505, unfollowed 3xx) as non retryable.
- Connection errors, timeouts (including `http.Client.Timeout`) and body read failures on a 2xx are
  all retryable. A transport error that happens because the context was cancelled is reported as
  cancelled, not retried.
- Exhausted error: `StatusCode` holds the last response status (0 when the last attempt had no
  response) and `Err` holds the last underlying error; both appear in `Error()`.
- Profile JSON shape assumed: `{"id": int, "name": string, "email": string}`.

## Bugs caught by the loop

None. Run 1 passed build, vet and all tests.

One bug was caught before the first run by reading my own code (not by the loop): the jitter
computation converted a float to `time.Duration` by truncation, so `100ms * 0.8` could become
79.999999ms because 0.2 is not exact in binary. I added `math.Round` before running anything. Since
it was never executed, it is not counted in BUILD_LOG.md.
