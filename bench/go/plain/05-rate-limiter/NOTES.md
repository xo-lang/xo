# Notes

## Ambiguities resolved

- Idle time: a bucket's last refill time is updated on every request (refill runs before every take), so it doubles as the last-seen time. "Idle for more than 10 minutes" is strict: exactly 10 minutes is kept, 10 minutes plus 1ns is evicted (tested).
- Retry-After: computed as ceil((1 - tokens) / refill_per_sec), clamped to at least 1. A 1e-9 tolerance is subtracted before ceil so float noise (for example 4.0000000001) does not round up an extra second. A bucket with refill 0 reports MaxInt32 seconds rather than dividing by zero.
- Denied requests do not consume anything; they only refill and compute Retry-After.
- Remote IP fallback strips the port from RemoteAddr (so one client on several ports shares a bucket); if RemoteAddr has no port it is used as is. X-Forwarded-For is not trusted since the task did not ask for it.
- New keys start with a full bucket. Custom limits apply on bucket creation, keyed by the same client key (API key or IP).
- "Stop when the server stops": the evictor goroutine is driven by a context; `Run` cancels it when its context is cancelled or the listener fails, shuts down the HTTP server, and waits for the evictor to exit before returning.
- Clock injection: the limiter takes a `func() time.Time`; the evictor ticker uses a real `time.Ticker` (interval configurable) but evicts using the injected clock, so the eviction test advances fake time and uses a 5ms ticker.
- Response body for 429 is plain text "rate limit exceeded"; only the status and header were specified.

## Bugs caught by the loop

None. The first run of build, vet, and test passed, so the loop caught no bugs. Any bugs present are ones the tests do not exercise.
