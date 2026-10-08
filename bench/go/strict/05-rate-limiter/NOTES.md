# Notes

## Ambiguities resolved

- Client key namespacing: ClientKey returns "key:<X-Api-Key>" or "ip:<remote IP>" so an API key that happens to look like an IP never shares a bucket with that IP. The custom limit config map uses raw values (an API key or an IP); the prefix is stripped before lookup.
- Remote IP: taken from `r.RemoteAddr` via `net.SplitHostPort`; if there is no port, the whole RemoteAddr is used. Proxy headers (X-Forwarded-For) are deliberately not trusted.
- Retry-After: ceil((1 - tokens) / refill_per_sec), minimum 1, with a 1e-9 epsilon so float noise does not round 2.0000001 up to 3. Computed after the refill done in take, under the same lock.
- Idle: a bucket is idle when no request has hit it for more than 10 minutes (strictly greater), tracked with a lastSeen time separate from the bucket's lastRefill.
- Clock going backwards: refill adds nothing and keeps the later lastRefill.
- Limits are validated: capacity must be finite and >= 1, refill must be finite and > 0 (otherwise a bucket could never grant a token or Retry-After would be infinite). NewLimiter returns an error otherwise.
- 429 body is JSON `{"error":"rate limit exceeded"}`; the task only specified status and header.
- Evictor lifecycle: `Serve(ctx, ln, limiter)` runs the HTTP server and the evictor goroutine (ticker every minute) and on ctx cancel shuts the server down and waits for the evictor to exit before returning. `RunEvictor` takes a tick channel so tests drive it without real time.
- Only `GET /hello` is routed; the limiter wraps the whole mux, so even 404/405 requests consume tokens.

## What each tool caught

- go test: the custom limit lookup bug (config map keyed by raw API key, lookup used the namespaced "key:" form). Caught by TestRetryAfter and TestCustomLimit. A real bug.
- nilaway: unchecked `resp` from `http.Get` in a test. The standard library guarantees a non-nil response when err is nil, so this is effectively a false positive in practice, but adding `resp == nil` to the check is cheap so I fixed it rather than suppressing.
- vet, staticcheck, errcheck, exhaustive, race: no findings.

## Suppressions

- None. No `//nolint` comments. One discarded value: `host, _, err := net.SplitHostPort(...)` discards the port, which is not an error value.
