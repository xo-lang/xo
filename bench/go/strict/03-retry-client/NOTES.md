# Notes

## Process notes

- `go.mod` was written by hand (`module benchtask`, `go 1.26`) instead of running `go mod init`, so that no command ran before the full solution was written. The result is identical to what `go mod init benchtask` produces.
- Before run 1 I made two pre-run edits: one small `python3` text substitution on retry.go (replaced a hacky `limit == maxSnippetBytes` check with an explicit `truncate` parameter) and an Edit removing leftover scaffolding in `TestJitterBounds`. No compiler or checker ran before run 1.

## API shape

- `GetWithRetry(ctx, doer Doer, clock Clock, rnd Rand, rawURL string, policy RetryPolicy) ([]byte, error)`. `net` became a `Doer` interface (`*http.Client` satisfies it), `clock` is `Clock.Sleep(ctx, d) error`, `rand` is `Rand.Float64()` (`*math/rand/v2.Rand` satisfies it). A `context.Context` was added as the idiomatic Go carrier for cancellation, which the spec's "cancelled" error kind needs.
- Errors are always `*FetchError` with a `Kind` (`KindInvalidURL`, `KindNonRetryableStatus`, `KindAttemptsExhausted`, `KindCancelled`), plus `Status`, `Attempts`, and `Err` (unwrappable). HTTP failures are wrapped as `*StatusError{Code, Body}`.
- `FetchProfile(ctx, doer, clock, rnd, baseURL, userID int64, policy) (Profile, error)` returns `*ProfileError` with `Kind` `ProfileErrFetch` (wraps the `*FetchError`) or `ProfileErrDecode` (wraps the `encoding/json` error).

## Ambiguities resolved

- Jitter: delay = base +/- `jitter * base`, computed as `base * (1 + jitter*(2r-1))` with r from `Rand`. Jitter is applied after the `max_delay` cap, following the spec wording literally ("plus or minus up to jitter of that value"), so a jittered delay can exceed `max_delay` by up to 20%.
- Retry-After: only integer seconds are supported (the spec says "in seconds"); HTTP-date values fall back to normal backoff. Retry-After delays are capped at `max_delay` and get no jitter. Retry-After is honoured only on 429 and 503, as specified; on 500 it is ignored (tested).
- Non 2xx statuses other than 429/500/502/503/504 (all other 4xx, and also 5xx like 501, and 3xx that the client did not follow) are treated as non retryable. The spec only says "other 4xx"; failing fast on unknown statuses seemed the safer default.
- Connection errors and timeouts: any error from `Doer.Do` is retried unless the context is done, in which case the result is `KindCancelled`. Per attempt timeouts are the Doer's job (for example `http.Client.Timeout`).
- Errors while reading a 2xx body are retried like connection errors. A body larger than 32 MiB is reported as an error and also retried (a deterministic failure retried needlessly; acceptable for this scope).
- `Attempts` in `KindCancelled` counts requests actually sent.
- Policy values out of range are clamped: `MaxAttempts < 1` becomes 1, negative delays become 0, jitter clamped to [0, 1]. `DefaultRetryPolicy()` supplies the documented defaults; a zero `RetryPolicy` is not silently promoted to defaults.
- URLs must parse, use http or https, and have a host; otherwise `KindInvalidURL`. Error messages use `url.URL.Redacted()` so passwords in URLs do not leak.
- `user_id: Int` became `int64`; the profile URL is `baseURL/users/{id}`, and the JSON shape `{"id","name","email"}` is my invention.

## What each tool caught

- nilaway (run 1): `resp.Body` dereference when an arbitrary `Doer` returns `(nil, nil)`. For `*http.Client` this cannot happen, but the code accepts any `Doer` interface, so it was a real robustness gap, not a false positive. Fixed by converting a nil response with nil error into `errNilResponse`, treated as a transport failure.
- vet, staticcheck, errcheck, exhaustive, race, tests: nothing found.

## Suppressions and discarded errors

- No `//nolint` comments and no `_ =` discards.
- Error response bodies: if reading the diagnostic snippet of a non 2xx body fails, the snippet is dropped (`snippet = nil`). The status code is what matters there.

## False positives

- None judged as false positives (see the nilaway note above).
