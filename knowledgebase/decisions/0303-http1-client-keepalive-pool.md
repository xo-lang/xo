# 0303: The native HTTP/1.1 client keeps connections alive in a pool (Go's http.Transport)

Status: Accepted (lead, 2026-10-21, within decision 0276: std follows
Go's standard library; Xo Lang 3)

## Context

The native std/http client (std/native/llrt hclient.xo and http1.xo
client) opened a connection for every HTTP/1.1 request and closed it
after the response. The Go backend's client is Go's http.Transport,
which keeps idle connections and reuses them. On bench/perf/httpclient's
HTTP/1.1 workload about 40% of the native client's CPU went to connect,
socket, close, shutdown and socket options (XO_PROF=cpu, 2026-10-21),
and each request paid a round trip for the handshake (two for TLS).
HTTP/2 connections were already pooled (hclient.xo pool_take and
pool_put, Transport's rules).

## Decision

The native client keeps HTTP/1.1 connections, plain and TLS, as
http.Transport does with its defaults:

- **Pool.** Idle connections live in the client pool HTTP/2 uses (one
  list, a kind per entry), keyed by scheme, address (host:port), TLS
  server name, and roots. At most 2 idle connections per key
  (MaxIdleConnsPerHost), 100 in all (MaxIdleConns; the oldest goes),
  each closed after 90 s idle (IdleConnTimeout). A connection whose
  TLS handshake chose HTTP/1.1 is pooled as HTTP/1.1.
- **When a connection goes back.** After a response read in full: its
  body was framed (Content-Length or chunked; a body ending at EOF
  closes), nothing was left unread after it, the status was not 101,
  and the response did not say close (HTTP/1.1 with `Connection:
  close`; HTTP/1.0 without `Connection: keep-alive`, Go's shouldClose).
  A `Connection: close` field in the request's header does not count:
  Transport's read loop looks only at Request.Close, which std/http's
  requests do not set (the Go backend reused the connection in golden
  http_client_keepalive); a server that honors the field closes, and the
  next request's take notices. The
  native client reads every body in full before it returns, so the
  "body read or closed before reuse" rule always holds.
- **Taking one.** A request takes the newest idle connection for its key
  that the server has not closed: one with input waiting (EOF, or bytes
  no request asked for) closes instead, as Transport's background read
  would have noticed.
- **A dead pooled connection.** When a reused connection fails before any
  byte of the response (the write fails, or the read gets EOF or a
  reset first), a replayable request (GET, HEAD, OPTIONS, TRACE, or one
  with an Idempotency-Key or X-Idempotency-Key field: Go's isReplayable,
  whose body condition always holds for std/http's in-memory bodies) is
  sent once more on a new connection, as Transport's retry does; any
  other request gets the error. (Go also replays any request whose
  write failed before a byte went out; the native client cannot tell a
  partial write from none and replays only replayable requests.)
- **Exit.** The leak check's exit hook (cabi_http.xo streams_free, via
  hclient.pool_free) closes the idle HTTP/1.1 connections with the
  HTTP/2 ones.

The request and response bytes do not change: the client already sent
no `Connection` field of its own (as Go's Transport sends none).

## Consequences

- A program making many requests to one server saves a connection setup
  (and a TLS handshake) per request; the client bench's HTTP/1.1
  workload measures it.
- Servers see one connection serve several requests, as with the Go
  backend; a server that closes idle connections is handled by the
  check on take and the replay of replayable requests.
- Behavior differences between the backends shrink: TestHTTP1ClientDiff
  keeps holding every backend to Go.

## Alternatives

- **Keep a connection per request**: simplest, but leaves the cost above
  and differs from the Go backend and from net/http.
- **A pool of its own for HTTP/1.1**: duplicates the HTTP/2 pool's
  locking, idle timeout and exit cleanup; one list with a kind per
  entry shares them.
- **Pool without the take check**: a server's idle close would turn the
  next request into a retry (or an error for a POST) every time.
