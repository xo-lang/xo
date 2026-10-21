# 0306: http.serve_content (Go's ServeContent)

Status: Accepted (owner, 2026-10-21, within decision 0276: std follows
Go's standard library; Xo Lang 3)

## Context

Go's http.ServeContent answers a request for a resource from its bytes:
Range requests (one range, or several as multipart/byteranges),
conditional requests (If-Match, If-Unmodified-Since, If-None-Match,
If-Modified-Since, If-Range) against the modification time and the
ETag the caller set, the Content-Type from the name's extension or the
content (DetectContentType), Last-Modified, Accept-Ranges, and its
error answers (416 with Content-Range, 412, 304). std/http had none, so
a file or blob handler had to reimplement them.

## Decision

`http.serve_content(req: Request, name: Str, modtime: Time?, content:
Bytes, headers: Map[Str, Str] = {}) -> Response` returns the response Go's
ServeContent writes for the same request, as a value:

- `headers` are the response fields the handler would have set on Go's
  ResponseWriter before the call (ETag, Content-Type, Cache-Control,
  Content-Encoding...): ServeContent reads ETag for the preconditions
  and Content-Type instead of guessing, and keeps the others.
- `modtime` None is Go's zero time (no Last-Modified, no date
  preconditions); `name` only picks the type by its extension.
- The body is the content, the range, or the multipart/byteranges body
  (its boundary random, as multipart.Writer's); HEAD gets the fields and
  no body.
- Errors are Go's: 416 "invalid range: failed to overlap" with
  `Content-Range: bytes */size` (an empty content ignores the range),
  416 for a malformed range, 412, 304 (no Content-Type or
  Content-Length, Last-Modified kept); error answers drop Cache-Control,
  Content-Encoding, ETag, and Last-Modified (Go's default, without the
  httpservecontentkeepheaders GODEBUG).
- The Go backend calls net/http's ServeContent itself (a recorder as the
  ResponseWriter); the native backends port fs.go's serveContent,
  checkPreconditions, the ETag scanners, and parseRange.

## Consequences

- File and blob handlers get Range and caching behavior from one call,
  the same on every backend.
- The content is in memory (Bytes): serving a large file is a later
  `serve_file`/streaming variant over a reader, not this function.

## Alternatives

- **A `Response` builder method** (`resp.serve_content(req)`): reads
  less like Go and still needs the request and the preset fields.
- **A reader instead of Bytes**: Xo has no seekable reader type in
  std/http yet; Bytes covers the common case (embedded files, blobs).
