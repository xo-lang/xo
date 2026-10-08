# Task 05: Rate limiter

Write a token bucket rate limiter and use it as HTTP middleware.

## Requirements

- `Bucket` holds `capacity`, `tokens` (fractional), `refill_per_sec`, and the
  last refill time. `take(now) -> Bool` refills based on elapsed time, then
  consumes one token if available.
- `Limiter` keeps one bucket per client key (the `X-Api-Key` header, or the
  remote IP if missing). Safe under concurrent requests.
- Default limit: capacity 20, refill 5 per second. Specific keys can have
  custom limits from a config map.
- Middleware: on limit exceeded, respond 429 with a `Retry-After` header (whole
  seconds until one token is available, at least 1).
- A background task evicts buckets idle for more than 10 minutes, every minute.
  It must stop when the server stops.
- Server: `GET /hello` returns `{"msg": "hi"}` behind the limiter.

## Tests to write

- 20 immediate requests succeed and the 21st gets 429.
- After 1 second on a fake clock, 5 more succeed.
- Retry-After value is correct.
- Custom limit for a key is applied.
- Idle buckets are evicted.
