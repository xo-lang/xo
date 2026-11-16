# 0674: http.encoded_responses, response compression with an encoding from outside std/http

Status: Accepted (lead for the owner, 2026-11-16 overnight; proposed by Xo Lang 3, 2026-11-16, task #89, for decision 0666's `brotli.responses`)

## Context

Decision 0666 gives std/brotli `responses(min_size, quality)`, the twin
of `http.gzip_responses` (decision 0305), so that
`r.wrap(brotli.responses()).wrap(http.gzip_responses())` serves `br` to
clients that accept it and gzip to the rest (andybalholm's
HTTPCompressor). std/brotli cannot build it today:

- std/http is in what compiler/xo builds from (`xo build --list-inputs`),
  and 0666 keeps the 7,000 line brotli port out of that closure, so
  std/http cannot import std/brotli.
- gzip_responses compresses a streamed body through BodyWriter's private
  `gz: Mutex[compress.Compressor]?` field (std/native/http.xo
  gzip_stream; std/rt/gzipresp.go on the Go backend). From outside
  std/http a middleware can neither reach a stream's function nor wrap
  its BodyWriter, so a std/brotli middleware could compress only buffered
  bodies.

## Decision

std/http gains one public middleware constructor and the type it takes:

```
/// A stream encoder for a streamed response body: each call returns the
/// encoded bytes it made (std/compress's Compressor and std/brotli's
/// Compressor have this shape).
struct BodyCoder {
  write: fn(Bytes) -> Bytes
  flush: fn() -> Bytes
  finish: fn() -> Bytes
}

/// A middleware that encodes responses as gzip_responses does, with the
/// Content-Encoding `token`: the same rules (status, an existing
/// Content-Encoding or Content-Range, already compressed types, Vary,
/// weak ETags, min_size for a buffered body, no sniffing of a stream),
/// when Accept-Encoding allows `token` (or `*`) with q > 0. `whole`
/// encodes a buffered body; `stream` makes a coder for each streamed
/// response, whose writes go through it (flush after its flush).
fn encoded_responses(token: Str, min_size: Int, whole: fn(Bytes) -> Bytes, stream: fn() -> BodyCoder) -> fn(Request, Next) -> Response
```

- gzip_responses becomes `encoded_responses("gzip", ...)` with
  std/compress's gzip on every backend (one set of rules); BodyWriter's
  private `gz` hook holds a BodyCoder instead of a gzip Compressor.
- std/brotli's `responses(min_size = 1024, quality = 5)` is
  `http.encoded_responses("br", min_size, ...)` with `encode` and a
  `brotli.Compressor` per stream, in std/brotli (pure Xo, every backend).
- Tests: http's gzip goldens unchanged (same bytes, headers and flush
  points); a `br` golden that serves buffered and streamed bodies,
  stacked with gzip_responses, decoded by Go's andybalholm Reader in the
  corpus oracle and by `brotli.decode`.

## Consequences

- Any encoding a program or package has (zstd later) plugs into
  std/http's rules without a change to std/http.
- std/http's surface grows by one type and one function; the closure
  does not grow (brotli stays outside).
- A BodyCoder's closures share state the middleware creates per
  response, so the coder is never shared between responses.

## Alternatives

- Put br in std/http: brings the brotli port into xo's build closure,
  against decision 0666.
- A buffered-only `brotli.responses` with public API alone: streamed
  responses (SSE, std/ui's streamed pages) would go out uncompressed or
  only as gzip; not HTTPCompressor's behavior.
- Expose BodyWriter wrapping (a public BodyWriter constructor from a
  write function): more surface (trailers, flush, connection state) than
  an encoder needs.
