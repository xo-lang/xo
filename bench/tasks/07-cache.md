# Task 07: Loading cache

Build a generic in memory loading cache and put it in front of a slow
upstream service.

## Cache

- `Cache[K, V]` created with a loader `load(key) -> V or error`, `max_entries`,
  `ttl`, `stale_ttl`, `negative_ttl`, and `load_timeout`.
- `get(key)`:
  - Fresh entry (younger than `ttl`): return it.
  - Stale entry (older than `ttl` but younger than `ttl + stale_ttl`): return it
    immediately and start one background refresh for that key (never more than
    one at a time per key).
  - Missing or expired: load it. Concurrent `get` calls for the same key share
    one load (single flight) and all receive its result.
- Loader errors are cached for `negative_ttl` and returned to callers during
  that time without calling the loader.
- A loader that exceeds `load_timeout` fails all waiters with a timeout error.
- A loader that panics (or crashes) fails all waiters with an error; the cache
  stays usable.
- `invalidate(key)` removes the entry. If a load for that key is in flight,
  its result must not be stored (it may still be returned to the callers that
  were already waiting).
- LRU eviction when `max_entries` is exceeded.
- `metrics()`: hits, stale hits, misses, loads, load errors, evictions.

## Service

- `GET /items/{id}` returns the item from the cache, backed by an upstream
  client that takes about 200ms per call (fake it in tests).
- `DELETE /items/{id}/cache` invalidates.
- Upstream 404 becomes 404 (and is negatively cached); upstream failure
  becomes 502.

## Tests to write

- 100 concurrent gets for one missing key call the loader exactly once.
- TTL expiry and stale while revalidate with a fake clock; exactly one background refresh.
- LRU evicts the least recently used entry.
- Invalidate during an in flight load: the load's result is not cached.
- Negative caching of errors.
- Loader timeout and loader panic fail all waiters; the cache works afterwards.
