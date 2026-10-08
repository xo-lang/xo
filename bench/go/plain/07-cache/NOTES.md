# Notes

## Layout

- `cache/`: generic `Cache[K, V]` (`New`, `Get`, `Invalidate`, `Metrics`, `Len`).
- `service/`: HTTP server (`GET /items/{id}`, `DELETE /items/{id}/cache`) plus `HTTPUpstream` client.
- `cmd/itemcache/`: small runnable binary wiring the two together.

## Ambiguities resolved

- Loader signature: `func(ctx context.Context, key K) (V, error)`. The context carries `LoadTimeout` so well behaved loaders can stop early; loaders that ignore it are abandoned (their goroutine finishes later, its result is discarded).
- `Get` takes a `context.Context`. A caller whose context ends stops waiting, but the shared load keeps running for other waiters and is still stored.
- Freshness boundaries: fresh while `age < ttl`, stale while `age < ttl + stale_ttl`, otherwise expired. `stale_ttl = 0` disables stale while revalidate.
- Negative caching: only errors returned by the loader are cached. Timeouts (`ErrLoadTimeout`) and panics (`*PanicError`) are treated as transient and never cached, so "the cache works afterwards" means the very next get retries. An optional `CacheError` predicate narrows which loader errors are cached; `negative_ttl = 0` disables negative caching.
- Service: only upstream 404 is negatively cached (the spec says so for 404 and the predicate is used for that); other upstream failures and timeouts map to 502 and are retried next request.
- Background refresh failure: a cacheable error (for example 404) replaces the stale entry with a negative entry; a non cacheable error keeps serving the stale value until it fully expires.
- Single flight is unified with background refresh: at most one load per key at any time, whether foreground or background. A get on an expired key while a refresh runs joins that refresh.
- Invalidate during an in flight load: the call is marked invalidated and detached from the in flight map, so its waiters still get its result, it is not stored, and a new get starts a fresh load whose result cannot be overwritten by the old one.
- Timestamps: an entry's age is measured from when its load finished (clock read at store time).
- Metrics definitions: hits = fresh hits including served negative entries; stale hits; misses = gets that waited on a load (including joiners); loads = loader invocations; load errors = failed loads (error, timeout, panic); evictions = LRU evictions only (not invalidations or expiry).
- LRU counts negative entries toward `max_entries`; any get that serves an entry (fresh, stale or negative) marks it most recently used.
- The "about 200ms" upstream is the real `HTTPUpstream` in production; tests use an in process fake with a gate channel (no real sleeps) and an `httptest` loopback server for the HTTP client.
- Time: an injectable `Now func() time.Time`; tests use a fake clock. The load timeout uses real time (100ms and 20ms in tests) because it is a wall clock bound on a goroutine.
- "Crashes": in Go, a loader panic is recovered in the loader goroutine and turned into `*PanicError`. A hard runtime fatal error cannot be recovered in process.

## Bugs caught by the loop

None. The first run of build, vet and test passed (see BUILD_LOG.md). No problems were found by the loop, so it caught no bugs; correctness rests on the test suite as written (single flight with 100 gets, stale while revalidate with exactly one refresh, TTL expiry, LRU order, invalidate during load including a newer load not being overwritten, negative caching, timeout with a ctx ignoring loader, panic, context cancellation, and the HTTP mapping). The race detector was not part of the prescribed loop and was not run.
