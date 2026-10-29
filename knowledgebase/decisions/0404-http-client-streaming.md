# 0404: the HTTP client streams bodies, takes a redirect policy and a timeout per request, and a reverse proxy builds on it

Status: Proposed (lead request, 2026-10-29: item 1 of the 0.35 std list;
Xo Lang 2). Design for review; no code yet.

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

**B. A streamed response body.**

```xo
fn (c: Client) open(req: Request) -> ClientStream ! ClientErr uses net.dial
struct ClientStream {}                                 // a response whose body is read as it arrives
fn (s: ClientStream) response() -> Response            // status and headers; body empty
fn (s: ClientStream) read(max: Int = 32768) -> Bytes? ! ClientErr  // the next bytes, None at the end
fn (s: ClientStream) read_all() -> Bytes ! ClientErr
fn (s: ClientStream) close()                           // read to the end: the connection is reused; else closed
```

`open` follows redirects as `send` does (each hop but the last read in
full and discarded, as Go's Client does). The body is gunzipped as it is
read on the same terms as `send` (compress's streaming Decompressor). A
stream is a handle like `File` and `Conn`: the program closes it, usually
`defer s.close()`; a task's end closes what it left open. The timeout
covers `open` and every `read`. Natively: `h1_round` and `h2_exchange`
split into "send and read the head" and "read the next body bytes"
(content length, chunked, until close; HTTP/2 DATA frames with flow
control), hclient's pool taking the connection back at `close`.

**C. A streamed request body.**

```xo
fn (r: Request) with_body_reader(b: BodyReader) -> Request  // a handle_stream route's body, sent as it is read
fn (r: Request) with_body_file(f: File) -> Request          // a file's bytes from its position
```

Sent with `Content-Length` when the request sets it, else chunked
(HTTP/1.1) or as DATA frames (HTTP/2). Such a request is not replayed:
a redirect that would resend its body (307, 308) returns the 3xx, and a
dead pooled connection fails the request instead of retrying it (Go's
`GetBody` absent).

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
