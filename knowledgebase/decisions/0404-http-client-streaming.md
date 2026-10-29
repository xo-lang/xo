# 0404: the HTTP client streams bodies, takes a redirect policy and a timeout per request, and a reverse proxy builds on it

Status: Proposed (lead request, 2026-10-29: item 1 of the 0.35 std list;
Xo Lang 2). Reviewed by Xo Lang 3 and the lead (2026-10-29). Stage A
landed in 440f5b333; B1 (HTTP/1.1) and B2 (HTTP/2) landed 2026-10-29; C's with_body_reader
landed 2026-10-29, with_body_file deferred (below); D landed 2026-10-29 as a
ReverseProxy value (below).

## Context

The 2026-10-29 std survey (knowledgebase/infrastructure/std-coverage.md,
"Survey of backend gaps") ranked the HTTP client first: an API gateway
written from spec/std.md could not stream a response or a request body,
could not pass a redirect through, and could not proxy at all except by
buffering. std/http's Client (spec/std.md 4.5) has `get`, `send` and
`post_json` over one `timeout` per client; a Response's `body` is the
whole body as `Bytes`, and the client follows up to 10 redirects for GET
and HEAD with no way to stop it. Go's net/http gives `Response.Body` (an
io.ReadCloser read as it arrives), `Request.Body` (an io.Reader sent as
it is read), `Client.CheckRedirect` (`ErrUseLastResponse` stops), a
context deadline per request, and net/http/httputil's `ReverseProxy`.

Natively the client is std/native/http.xo `Client.send` over
std/native/llrt/http1.xo `client_do`, which runs the redirects, one
exchange per hop over hclient.xo's pooled HTTP/1.1 or HTTP/2 connection
(`h1_round`, `h2_exchange`), and returns the whole body, gunzipped as
Transport does. The Go backend's client is std/rt/http_net.go over Go's
own net/http.

## Decision (proposed)

In stages, each with Go's tests ported where they fit (net/http
client_test.go and transport_test.go: redirects, timeouts, bodies
streamed and closed early, gzip) and a timing row against a Go twin.

**A. A redirect policy and a timeout per request.**

```xo
fn (c: Client) with_redirects(max: Int) -> Client   // 0: none followed, the 3xx is the Response; default 10
fn (r: Request) with_timeout(d: Duration) -> Request  // this request's limit, in place of the client's
```

Go's general CheckRedirect callback is not offered: a maximum covers the
proxy (0) and the default (10), and a function value in Client would make
it a non-data value. `ClientErr.Connect` is documented as "the request
did not reach the server" (no connection or no TLS handshake: safe to
retry any method), `Timeout` and `Protocol` as possibly having reached it.

**B. A streamed response body.** (Revised 2026-10-29 after Xo Lang 3's
review and the lead's: the handle with `close`, as File and Conn.)

```xo
fn (c: Client) open(req: Request) -> ClientStream ! ClientErr uses net.dial
struct ResponseHead { status: Int, headers: Map[Str, Str] }   // with header and header_values as Response's
struct ClientStream {}                                   // a response whose body is read as it arrives
fn (s: ClientStream) head() -> ResponseHead
fn (s: ClientStream) read(max: Int = 32768) -> Bytes? ! ClientErr uses net.dial  // the next bytes, None at the end
fn (s: ClientStream) read_all(max_size: Int = 1073741824) -> Bytes ! ClientErr uses net.dial
fn (s: ClientStream) close()                             // as Conn.close: no error, again does nothing
fn (r: Request) with_idle_timeout(d: Duration) -> Request    // open: the head within d, then each read within d
```

- Capabilities mirror Conn: `open`, `read` and `read_all` use `net.dial`;
  a function handed a stream needs the capability to read it.
- The head is a type of its own, so no caller reads an empty `body`.
- `open` follows redirects as `send` does (A's policy); each hop's
  connection is released before the next.
- Timeouts: `send` and `get` keep a total deadline over the whole
  exchange (Go's Client.Timeout). A stream has no total deadline (it
  would cut a long download or an event stream): the head must arrive
  within the client's timeout (or `with_timeout`'s), and then each `read`
  within `with_idle_timeout`'s limit (the client's timeout when unset).
  A `Timeout` from `read` means the head had been received.
- Errors after `open` are never `Connect`: a reset or a read failure is
  `Protocol` with the transport's text, an idle limit `Timeout`, a
  cancelled task `Canceled`. A body cut short (end of stream before its
  Content-Length, a chunked body cut, an HTTP/2 stream reset) is
  `Protocol` (Go's io.ErrUnexpectedEOF), never a clean `None`.
- gzip: decoded as it is read on the same terms as `send` (only when the
  client added `Accept-Encoding` itself), through std/compress's streaming
  Decompressor with its 1 GiB `max_size` (decision 0305); `read_all`'s
  `max_size` bounds the rest of the body the same way.
- Release: the stream is a counted runtime handle (decision 0295). When
  its last reference goes (close, or the value dropped at its scope's end
  or its task's) the connection is returned to the pool if the body was
  read to its end, and otherwise closed (HTTP/1.1) or the stream reset
  with RST_STREAM CANCEL (HTTP/2, the connection staying pooled). `close`
  is idempotent.
- Natively `h1_round` and `h2_exchange` split into "send and read the
  head" and "read the next body bytes" (Content-Length, chunked, until
  close; HTTP/2 DATA frames with flow control), and `send` becomes
  `open` plus `read_all`, one path for both.
- Tests: Go's client_test.go TestClientRedirects and TestClientTimeout*,
  transport_test.go TestTransportBodyReadError and the early close cases
  (a stream closed or dropped before its end: its connection is not
  reused, no fiber leaks, under XO_LEAKCHECK), and a timing row against a
  Go twin: a 100 MB download streamed against `send`'s buffered one,
  wall time and peak RSS (flat for the stream).

**C. A streamed request body.**

```xo
fn (r: Request) with_body_reader(b: BodyReader) -> Request  // a handle_stream route's body, sent as it is read
fn (r: Request) with_body_file(f: File) -> Request          // a file's bytes from its position
```

Client side builders (a handler's own Request ignores them). Sent with
`Content-Length` when the request sets it, else chunked (HTTP/1.1) or as
DATA frames (HTTP/2). Such a request is not replayed: a 307 or 308 that
would resend its body returns the 3xx, and a dead pooled connection
fails the request instead of retrying it (Go's `GetBody` absent). A 301,
302 or 303 turns the request into a GET without a body, as `send` does.

Deferred (2026-10-29): `with_body_file`. A File's read needs `uses
fs.read`, and the body source is read later inside `send` (`uses
net.dial`), so the builder would hide an effect from the caller's
signature. Until a design keeps the effect visible (a `send` overload
taking the File, or a source type that carries its effects), a file is
sent with `os.fs.read` into `body`, or through a BodyReader.

**D. A reverse proxy.**

```xo
pub fn reverse_proxy(client: Client, target: Url, rewrite: fn(Request) -> Request = fn(r) { r }) -> fn(Request, BodyReader) -> Response
```

For a `handle_stream` route: the request is rewritten to the target
(path joined as httputil's `joinURLPath`, query merged), hop-by-hop
headers removed both ways (Connection and its list, Keep-Alive,
Proxy-*, TE, Trailer, Transfer-Encoding, Upgrade), `X-Forwarded-For`,
`-Host` and `-Proto` set as `ProxyRequest.SetXForwarded`, sent with the
body streamed (C) and redirects off (A), and the response streamed back
(B) through `http.stream`; a transport error is 502, a timeout 504, as
`ReverseProxy`'s default ErrorHandler. Not in D: Upgrade and WebSocket
tunnels (stage E, with a connection hijack), trailers, FlushInterval
(each read is written and flushed).


As built (2026-10-29): `reverse_proxy(client, target) -> ReverseProxy`,
`with_rewrite(f)` and `serve(req, body) -> Response uses net.dial`, in
place of a returned handler function: a function type with `uses` in a
return position reads as the outer function's clause, and a value with a
method keeps the effect on `serve`, as Go's `ReverseProxy.ServeHTTP`.
**E (later). Upgrade tunnels** for a proxied WebSocket, after a server
side hijack exists.

## Consequences

- A to D add API only: `send`, `get`, `post_json` and their behavior stay.
- The native transport's exchange splits in two; the whole-body path
  becomes `open` plus `read_all`, so one code path serves both.
- Each stage changes std/http's spec section 4.5 and std/changes/http.md
  in the same commit, on the native backends and the Go backend (Go's
  net/http underneath there).

## Alternatives

- A callback form, `c.with_stream(req, fn(Response, Body) -> T)`, which
  closes the stream when the function returns: no leak by construction,
  but a proxy must then write the response from inside the callback and
  the error types nest; the handle with `close` matches File and Conn.
- Go's CheckRedirect function: more general, rarely needed beyond "stop"
  and a count; can be added later as `with_redirect_check`.
- Streaming by default (Response.body lazy): changes every caller's
  behavior and error points.
