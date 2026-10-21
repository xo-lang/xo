# 0310: Streaming request bodies in std/http

Status: Accepted (owner, 2026-10-21, within decision 0276: std follows
Go's standard library; Xo Lang 3)

## Context

A std/http handler gets the whole request body in memory
(`Request.body`, read before the handler runs and capped by
`XO_HTTP_MAX_BODY`, 64 MiB). Go's handlers read `r.Body` as a stream, so
an upload larger than memory, a body processed as it arrives (a log
shipper, a proxy, multipart parts written to disk), or a handler that
answers before the client finished sending (413, 401) is possible
there and not in Xo.

## Decision

A route opts in to a streamed body, with a handler that takes it:

```xo
fn (var r: Router) handle_stream[E: Error, fx: Effects](pattern: Str, h: fn(Request, BodyReader) -> Response ! E uses fx) uses fx

struct BodyReader {}
fn (b: BodyReader) read(max: Int = 32768) -> Bytes? ! IoErr   // the next bytes (at most max), None at the end
fn (b: BodyReader) read_all() -> Bytes ! IoErr                // the rest
```

- The server matches the route from the request's head. For a
  `handle_stream` route it calls the handler before reading the body;
  `req.body` is empty and the BodyReader reads the body as the handler
  asks: Content-Length, chunked, or HTTP/2 DATA frames (the stream's
  flow control window grows as the handler reads, as net/http's).
  Other routes are unchanged (the body read first, in `req.body`).
- `XO_HTTP_MAX_BODY` does not apply to a streamed body (Go has no
  limit; the handler decides).
- After the handler returns, an unread rest of the body is discarded
  up to 256 KiB to keep the connection for the next request, else the
  connection closes, as net/http's server does.
- A read after the client went away, or a body cut short, is
  `IoErr` with net/http's texts (`unexpected EOF`).
- Middleware (`wrap`) runs as for any route; it sees the request
  without its body.
- `Router.call(req)` (in-process dispatch, tests) streams `req.body` to
  a `handle_stream` handler.

Phases:

1. The API, the Go backend (net/http's `r.Body` behind the reader; the
   route found before the body is read), the native HTTP/1.1 server,
   goldens against the Go backend.
2. The native HTTP/2 server (DATA frames into the reader, WINDOW_UPDATE
   as read).
3. Record and replay (decision 0219's request recording records the
   chunks read), and multipart parts read from the stream.

## Consequences

- Uploads and proxies no longer need the body in memory; the default
  stays the simple buffered `req.body`.
- The server's dispatch order changes for streamed routes only (match,
  then body).
- A streamed handler's request cannot be recorded for replay until
  phase 3; until then a recording server buffers it as before.

## Alternatives

- **Every body lazy** (`req.body` read on first use): keeps one handler
  type, but a struct field cannot block or fail, and every handler
  would pay for the laziness.
- **A Request method on every route** (`req.body_reader()` reading
  what the server already buffered): the body is still in memory; the
  point is not to read it first.
- **A flag on `handle`** (`stream_body: true` with the same handler
  type): the handler cannot tell from its type whether `req.body` is
  filled; a separate signature makes it explicit.
