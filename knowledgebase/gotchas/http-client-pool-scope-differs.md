# The connection pools differ by backend: native shares one per process, the go backend's expect requests pool apart

Status: Observed (2026-11-16, Xo Lang 3, writing golden http_client_trace for decision 0677)

## Symptom

A golden that traced an `Expect: 100-continue` POST (with_expect_continue)
after other requests to the same server printed `GotConn reused=true` on
arm64 and `ConnectStart ... GotConn reused=false` on the go backend. A
fresh `http.client(...)` for that request did not help: arm64 still reused
the earlier connection.

## Cause

- The native client keeps one idle pool per process (hclient's h2pool,
  keyed by address, server name, roots and HTTP/2 config), so every
  Client to the same server shares connections whatever Client sent them.
- std/rt's clients share `sharedTransport`, but a request with
  `with_expect_continue` goes through `expectTransport`, a Transport of its
  own (Go's ExpectContinueTimeout is per Transport), whose pool is empty.

Both are fine for users (pooling is not observable without tracing), but a
test that prints GotConn's reused, or counts accepted connections, sees the
difference.

## Fix

In a test, make the pool state the same on both: send the request after a
response that closed the connection and with `Connection: close` itself, so
both backends dial and neither pools it (http_client_trace does this).
Do not rely on a new Client for a fresh connection on the native backends.
