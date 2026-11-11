# 0607: http.file_server, Go's FileServer over an Fs

Status: Accepted (owner, 2026-11-11; Xo Lang 3)

## Context

The std audit of 2026-11-11, gap 10: there is no handler that serves a
directory. `serve_content` (decision 0306) serves one value with Go's
Range and conditional request handling. Six modules in the audit's
corpus write their own static file handler, and an `embed.Dir`'s `fs()`
was made an `Fs` so that such a handler can serve it (std.md 31).

## Decision

`pub fn file_server(fs: Fs, strip: Str = "") -> fn(Request) -> Response uses fs.read`
in std/http, Go's `FileServer` (with `StripPrefix(strip, ...)` folded in),
registered as any handler: `r.handle("GET /static/", http.file_server(os.fs.sub("public"), strip: "/static"))`.

- The path is the request's decoded path without `strip` (404 when it
  does not start with it, as StripPrefix), cleaned as `path.Clean("/" +
  p)`. It is then read through the Fs relative to its root. The Fs is
  the sandbox: a sub capability cannot be escaped (`..` is `Denied`), and
  an attenuated or embedded Fs serves only what it holds.
- Go's serveFile otherwise:
  - `.../index.html` redirects to `./` (301, local, the query kept).
  - A directory without a trailing slash redirects to `name/`; a file
    with one redirects to `../name`.
  - A directory serves its `index.html` when there is one. Otherwise it
    gets Go's listing: `<!doctype html>`, the viewport meta, `<pre>`,
    one `<a href>` per entry sorted by name, directories with `/`, names
    URL-escaped in the href and HTML-escaped in the text. It also gets
    `Last-Modified` and a 304 from If-Modified-Since.
  - A file goes to `serve_content` with its name, its modified time and
    its bytes.
  - Errors are Go's toHTTPError texts: `NotFound` is `404 page not
    found`, `Denied` is `403 Forbidden`, and any other error is `500
    Internal Server Error`, never the error's own text.
- The file is read whole, as `serve_content` takes Bytes. A streamed
  body (`File.read_at` in pieces with the Range) is a later stage. It
  needs `serve_content` over a reader, which is the same work Go's
  `io.ReadSeeker` does.
- The Go backend implements it over the Xo `Fs` too (std/rt), not over
  net/http's `FileServer`, so a user Fs, a MemFs and an embedded Dir
  answer alike. A corpus of requests is checked against Go's FileServer
  over the same tree (`os.DirFS` of a generated directory).

## Consequences

- Static sites and asset routes need no hand-written handler. An
  `embed.Dir` serves with `http.file_server(SITE.fs(), strip: "/static")`.
- Large files are held in memory per request until the streaming stage.
  std.md says so.
- Dotfiles are served like any file, as Go's FileServer does. A program
  that wants to hide them passes an attenuated Fs or wraps the handler.

## Alternatives

- `Router.static(prefix, fs)`: one call fewer, but it is a second way to
  register a route and hides the handler from middleware composition.
- Streaming from the start: it waits on a reader form of `serve_content`.
  This decision does not block that; the handler's signature stays.
- Using net/http's FileServer on the Go backend: it would need an
  `fs.FS` adapter over a user Fs, and its behavior would differ from the
  native port wherever the Fs differs from the OS.
