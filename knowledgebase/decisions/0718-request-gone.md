# 0718: Request.gone, the signal that a server request's client left

Status: Accepted (lead, 2026-11-18, xw question 153, option 2)

## Context

std/grpchealth's Watch handler registered a watcher and looped on its
channel until the service's status changed. A client that canceled its
Watch (an HTTP/2 RST_STREAM) left the watcher registered: the handler
learned of the reset only on its next write, which waited for a status
change that might never come. The #152 grpchealth stress program found it
(5 canceled watches, 5 watchers left). A handler had no way to learn that
its client was gone: Go's handlers select on `r.Context().Done()`, and
grpc-go's Watch returns on `stream.Context().Done()`.

## Decision

- `http.Request.gone() -> Chan[()]`: a channel closed when a server's
  HTTP/2 request's client is gone: its stream reset (RST_STREAM), a stream
  error, the connection's end (GOAWAY, close), or the stream freed after
  the handler. The connection's fiber closes it (std/native/llrt/h2srv.xo
  `gone_add`, `gone_close`, the stream record's S_GONE block, each channel
  holding a reference released at close).
- The channel is made with the request (std/native/http.xo
  `stream_request`) and kept in the request's hidden values, not a field: a
  Chan field would make every Request shared state for the effect checker.
- `grpc.Call.done()` gives the request's channel to gRPC handlers;
  grpchealth's Watch selects on it and its watcher channel.
- Never closed (a channel no one closes) for an HTTP/1.1 request, a
  request built in Xo code, and on wasm (compile-time target check).

## Consequences

- A streaming handler can stop work for a client that left, as in Go.
- On the Go backend (std/rt) the channel closes when net/http cancels
  the request's context (any protocol, as Go's Request.Context().Done()).
- HTTP/1.1 is a later stage: noticing a peer's close needs a read on the
  connection while the handler runs (Go's background read in
  `connReader.startBackgroundRead`).
- Each server HTTP/2 request allocates one unbuffered channel.

## Alternatives

- Option 1 (question 153): cancel the handler's task when its stream is
  reset or its connection closes (net/http cancels `r.Context()`).
  Rejected for now: every handler would be canceled at its next
  cancellation point, a behavior change for all of them.
- Option 3: grpchealth alone polls with a timeout and a liveness probe of
  the stream. Rejected: it fixes one handler and needs a new probe in
  h2srv's sink.
