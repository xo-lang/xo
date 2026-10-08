# Notes: 07-cache (strict Go)

## Layout
- `cache.go`: generic `Cache[K, V]` (`New`, `Get(ctx, key)`, `Invalidate`, `Metrics`, `Len`).
- `service.go`: `Upstream` interface, `HTTPUpstream` client, `Server` with `GET /items/{id}` and `DELETE /items/{id}/cache`.
- Tests: `cache_test.go` (fake clock, manual timeout timers), `service_test.go` (httptest upstream plus a gated fake upstream that stands in for the ~200ms call without sleeping).

## Ambiguities resolved
- `get` takes a `context.Context` (idiomatic Go). A caller whose context ends stops waiting with `ctx.Err()`; the shared load keeps running for other waiters.
- The loader receives a context that is canceled when `LoadTimeout` fires.
- Only errors returned by the loader are negatively cached. Timeouts and panics fail all current waiters but are not cached, so the next `get` retries ("the cache works afterwards"). All three count as `LoadErrors`.
- All loader returned errors are negatively cached, including upstream failures that become 502 (the spec says loader errors are cached; 404 is called out explicitly but 502 is not excluded).
- A failed background refresh keeps the stale value (it is served until `ttl + stale_ttl`, then a blocking load happens). Refresh errors are not cached.
- A get that finds the entry fully expired while a background refresh is still in flight joins that refresh rather than starting a second load.
- A hit on a negatively cached error counts as a hit. A get that joins an in flight load counts as a miss. `Loads` counts loader invocations, including background refreshes.
- `MaxEntries` 0 means unbounded. Cached errors occupy LRU slots like values.
- `Invalidate` removes the entry and detaches any in flight load: its waiters still receive its result, it is not stored, and the next `get` starts a new load.
- A loader that calls `runtime.Goexit` is treated like a panic (`ErrLoaderPanic`).
- Timeout and panic errors from the service map to 502 (upstream failure); only `ErrNotFound` maps to 404. `DELETE` returns 204.
- Timeouts use an injectable `After` function so the timeout test fires deterministically instead of waiting in real time.

## Bugs caught by tools
- None of the tools caught a real bug. The only findings (nilaway, run 1) were false positives.
- One real issue was found by self review before the first run: `t.Fatalf` reached from non test goroutines through the `doRequest` helper. It was fixed before any command ran, so no tool had the chance to report it.

## False positives
- nilaway, run 1: `http.Client.Do` result 0 flagged as possibly nil at three call sites (`service.go` FetchItem, `service_test.go` doRequest and fetchStatus). `Do` documents a non-nil response whenever err is nil. Fixed by adding explicit `if resp == nil` guards that return an error (or fail the test), not by suppression.

## Suppressions
- No `//nolint` comments and no `_ =` discards. In `fetchStatus` (test helper), the Close error is joined with a read error via `errors.Join`, not discarded.
