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
- The native server discards (or gives up on) an unread body when the
  handler returns, before a streamed response's function runs; net/http
  does it at the response's first write, so a `http.stream` function can
  still read the request body on the Go backend only.
- Phase 1 landed 2026-10-21 (std/rt http_body_reader.go, the native
  http1.body_next and body_finish, golden http_stream_body identical on
  both backends).
- Phase 2 landed 2026-10-21: the native HTTP/2 server (h2srv.xo) hands a
  handle_stream route's request over at its HEADERS (conn_streaming),
  keeps DATA in the stream record for body_next, gives the connection's
  window back at once and the stream's as the handler reads, and resets
  a stream whose body is still coming with NO_ERROR after the response
  (handler_done); the BodyReader holds its read and finish functions,
  so a plain server still links no HTTP/2 code (decision 0244). Golden
  http_stream_body_h2.

## Addendum 2026-10-21: phase 3, multipart parts from the stream

Owner-approved API (with the lead's change: keep repeated headers and
read like std/multipart's Part):

```
fn (b: BodyReader) multipart(req: Request) -> PartReader ! multipart.MultipartErr
struct PartReader {}
fn (p: PartReader) next() -> StreamPart? ! multipart.MultipartErr
fn (p: PartReader) read(max: Int = 32768) -> Bytes? ! multipart.MultipartErr
struct StreamPart { name: Str, filename: Str?, headers: Map[Str, Str] }
fn (p: StreamPart) header_values(name: Str) -> List[Str]
```

- `StreamPart` has `multipart.Part`'s fields but `data`, so the two
  read alike: `headers` holds each header's first value (as Part's),
  and `header_values` gives every value in order, as Go's MIMEHeader;
  std/http has no multi-value header type, and the hidden field list
  (`fields`, Go's `XFields`) is decision 0288's Request and Response
  pattern.
- Go backend: `multipart.NewReader` over the BodyReader's source
  (std/rt http_part_reader.go). Native: std/native/multipart.xo's
  reader takes its input from a source function and refills its buffer
  (`more`), so the in-memory parser and the stream share one port of
  Go's code; a quoted-printable part is decoded whole at its first read
  (Go decodes a line at a time; the bytes and the errors are the same).
  std/native/http reaches it through `use native/multipart` (decision
  0319).
- Errors are `MultipartErr` with Go's texts; a body read error becomes
  its text (`unexpected EOF`).
- Recording streamed bodies (decision 0219) stays deferred unless the
  owner asks.
- Golden http_multipart_stream: parts read 5 bytes at a time, repeated
  headers, a file name's path, quoted-printable data, skipped parts
  (one with a bad quoted-printable line, after which Go goes on),
  pipelined requests, a 70,000 byte part, bodies cut inside a part and
  inside the headers, a body sent 7 bytes per chunk, preamble and
  epilogue, both Content-Type errors, and Router.call; identical on
  the Go and native backends.

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
