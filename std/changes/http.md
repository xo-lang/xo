# std/http changes

## 2026-10-30: fix: a pipe write before the first read is held, every backend (decision 0444)

- Kind: fix
- Commit: this commit
- Found by: Xo lang 5's gRPC client (agent survey 2026-10-30, item 3: write the first message, then Client.open, hung)
- Go reference: io.Pipe (each Write waits for a Read); Xo departs from it before the first read
- Test: internal/llvm/testdata/golden/http_pipe ("prewrite": two writes and a close in the task that then opens the request; arm64, LLVM and the Go backend)
- Timing: none (the held pieces are the bytes the writer gave)

`PipeWriter.write` before the BodyReader's first read now holds the
piece and returns; the reader takes the held pieces first, then waits
on the writer as before. The Go backend waited forever on the first
such write and the native backends on the second.
Also native: a `Client.open` stream with a piped body, read to its end
without `close()`, kept its sending group live until exit (one
allocation under XO_LEAKCHECK); it now closes when the body ends or
fails, as Go's transport is done with the request then.

## 2026-10-30: fix: an undeclared trailer after a short streamed body, Go backend (decision 0433)

- Kind: fix
- Commit: this commit
- Found by: Xo lang 5's header and trailer probe across backends (agent survey 2026-10-30, item 3)
- Go reference: net/http's chunkWriter.writeHeader (a handler done within the buffer gets a Content-Length from the headers seen at WriteHeader)
- Test: internal/llvm/testdata/golden/http_trailers ("h1 short": `write("hi")` then `set_trailer("Grpc-Status", "0")`; the Go backend's output equal to the native one's)
- Timing: one flush when an HTTP/1.1 streamed response sets a trailer

On HTTP/1.1 the Go backend's server wrote a streamed response's status
before running its function, so net/http fixed the framing from the
headers it had then: a body short enough to end with the handler went
out with a Content-Length, and a trailer set without
`with_trailer_names` was dropped. The native server sends such a
response with a body and a trailer chunked and delivered the trailer.
The Go backend now flushes the head when a trailer is set after a body
byte (or after the next write, for one set first), so it goes out
chunked too and its trailers follow the last chunk; a streamed
response without trailers keeps its Content-Length, and one with no
body keeps Content-Length: 0 and drops undeclared trailers, as before.
Header and trailer names were already canonical on every backend, in
both directions and over a raw peer's lowercase names.

## 2026-10-30: feature: Client H2Config PING keepalive, native (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the client ignored send_ping_timeout and ping_timeout)
- Go reference: net/http's Transport.HTTP2.SendPingTimeout and PingTimeout ("http2: client connection lost")
- Test: internal/llvm/testdata/golden/http_h2_config ("client ping": a raw server never answers; it sees the client's PING and the request fails; native output equal to the Go backend's)
- Timing: none unless set (the read keeps its deadline)

While a request waits for the server's next frame, a read idle for
send_ping_timeout sends a PING, and a further silence of ping_timeout (0:
15 s) fails the request with "http2: client connection lost" and drops
the connection. Go pings from a reader of the connection even with no
request; the native client has no such task, so an idle pooled
connection is not probed.

## 2026-10-30: feature: Client H2Config.max_read_frame_size, native (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the native client always sent MAX_FRAME_SIZE=16384)
- Go reference: net/http's Transport.HTTP2.MaxReadFrameSize (the SETTINGS_MAX_FRAME_SIZE the transport sends and the frame size it accepts)
- Test: internal/llvm/testdata/golden/http_h2_config ("client tuned" now reads MAX_FRAME_SIZE=65536; native output equal to the Go backend's)
- Timing: none (one setting per connection)

The native client sends the configured size (16384 to 16777215, 0 the
default 16384) in its SETTINGS and accepts frames up to it; a larger
one is FRAME_SIZE_ERROR as before. A different size is a different
pooled connection.

## 2026-10-30: fix: a 431 or 413 reply survives unread request bytes on Linux, native server

- Kind: fix
- Commit: this commit
- Found by: TestGoldenRun/http_server_limits on the linux/arm64 host ("big header: error" where macOS got the 431)
- Go reference: net/http's closeWriteAndWait (CloseWrite, then a wait before Close)
- Test: internal/llvm/testdata/golden/http_server_limits (arm64, LLVM, Go parity), on the linux/arm64 host
- Timing: none (the lingering read is on the error path only)

The server answered 431 (head over with_max_header_bytes) and 413 (body
over with_max_body), shut down its sending side and closed with the
rest of the request still unread; on Linux that close sends a RST, which
can discard the reply from the client's receive buffer, so a client
still writing a 100 KB header saw a connection error. The server now
reads and discards what the peer still sends (at most 500 ms or 1 MiB,
until the peer closes) before it closes, as net/http does.

## 2026-10-30: feature: H2Config.max_encoder_header_table_size, native server (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the native server ignored the field)
- Go reference: net/http's http2.Server.MaxEncoderHeaderTableSize
- Test: http_h2_config still equal to want.txt (no probe: the cap only shows as a dynamic table size update in a response's first header block)
- Timing: none

The HPACK encoder of a connection keeps its dynamic table within the
setting (0 or over 4096: 4096) and the peer's SETTINGS_HEADER_TABLE_SIZE,
whichever is smaller; a cap below 4096 is announced with a table size
update in the first block, as Go.

## 2026-10-30: feature: H2Config.write_byte_timeout, native server (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the native server ignored the field)
- Go reference: net/http's http2.Server.WriteByteTimeout
- Test: http_h2_config still equal to want.txt; no stalled-reader probe (the kernel's socket buffers make when a write blocks depend on the host, so a golden would not be stable)
- Timing: none (the default writes as before)

With the timeout set, the HTTP/2 server writes its pending output in
16 KiB pieces that each must finish within it, and closes the connection
when one does not (no write progress). Unset: one write, no deadline.

## 2026-10-30: feature: Client.with_h2_config, native (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the 0434 open list (the client stored its H2Config and ignored it)
- Go reference: net/http's Transport.HTTP2 (HTTP2Config): MaxReceiveBufferPerStream, MaxReceiveBufferPerConnection, MaxDecoderHeaderTableSize; the SETTINGS and WINDOW_UPDATE net/http's HTTP/2 transport opens a connection with
- Test: internal/llvm/testdata/golden/http_h2_config (a raw server reads a client's first frames; native output equal to the Go backend's)
- Timing: none (settings written once per connection)

The native client sends its H2Config in the connection's SETTINGS
(INITIAL_WINDOW_SIZE, HEADER_TABLE_SIZE when not 4096) and connection
WINDOW_UPDATE, sizes its HPACK decoder and flow windows by it, and keeps
clients with different configs on different pooled connections. The
default connection now opens as net/http does: MAX_FRAME_SIZE=16384 in
the SETTINGS, and a WINDOW_UPDATE of the whole 1 GiB (the window was
the same 1 GiB less 65535 before: now 65535 above it, as Go). Not applied
by the native client: max_read_frame_size (its frames stay 16 KiB),
the PING keepalive and write_byte_timeout. The golden also pins max_concurrent_streams: a second stream while
one is open gets RST_STREAM REFUSED_STREAM, as Go.

## 2026-10-30: feature: HTTP/2 client full duplex, native (decision 0433)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-5's gRPC interop program (ping_pong deadlocked: "every task is blocked")
- Go reference: net/http's HTTP/2 transport writes the request body from its own goroutine while the response is read
- Test: internal/llvm/testdata/golden/http_duplex (an h2c row next to the h1 one); the gRPC interop client against the Xo server, 9 of 9 cases
- Timing: none on a request without a piped body

`Client.open` with a piped body (`http.pipe`) over HTTP/2 returns at the
response head while the body is still written. The h2 pump pulled the
body source with a blocking read, which waited for a message that the
program writes only after a response arrives. A feeder task now pulls
the source into a queue, and the pump polls the queue and reads the
socket with a 10 ms deadline while it is empty (the request's own
deadline is put back).

## 2026-10-30: feature: h2c and HTTP/2 settings, native (decision 0434)

- Kind: feature
- Commit: this commit
- Found by: the gRPC readiness survey of 2026-10-30, gap 10
- Go reference: net/http's Protocols (UnencryptedHTTP2), HTTP2Config (MaxConcurrentStreams, MaxReadFrameSize, MaxReceiveBufferPerConnection/Stream, MaxDecoderHeaderTableSize, SendPingTimeout, PingTimeout)
- Test: internal/llvm/testdata/golden/http_h2c, http_h2c_sniff, http_h2_config (native output equal to the Go backend's)
- Timing: none on the request path (a 24 byte preface peek on a plain connection that allows h2c); `xo bench --ab` on the linux/arm64 host (OFF_EXCLUSIVE, 10 rounds, bench/perf/h2loop: 3000 h2 TLS requests over 200 connections, base c51c9f4f1 and the per-connection settings 15febcd33): wall 226.7 to 227.6 ms (1.004, interval 0.993 to 1.015), CPU 225.0 to 230.0 ms (1.022), max RSS 44.5 to 45.7 MB (1.027), all within noise

A plain server with `UnencryptedHttp2` in `with_protocols` sniffs the
HTTP/2 preface and serves h2c next to HTTP/1.1 (h2c only: other
connections close); a client with it speaks prior knowledge, and its
idle pooled connections are kept apart from HTTP/1.1 ones. ALPN follows
the protocol sets on both sides (an h2 only TLS server closes a
connection that did not negotiate h2). `with_h2_config` sets each
server connection's settings, and `send_ping_timeout` / `ping_timeout`
send a PING after that much read silence and close when no ack follows.
Requests on h2c report `tls() == false` and scheme `http`.

## 2026-10-30: feature: protocol sets and HTTP/2 settings (with_protocols, with_h2_config, h2c)

- Kind: feature
- Commit: this commit
- Found by: decision 0434 (gRPC behind a mesh sidecar speaks h2c with prior knowledge)
- Go reference: net/http Protocols, Server.Protocols, Transport.Protocols, HTTP2Config (go1.26.1)
- Test: golden http_h2c (an h1 + h2c server answering both clients on one port, an h2c only server, ALPN with one protocol removed, an empty set)

`Server.with_protocols` and `Client.with_protocols` take a set of
`Protocol` (`Http1`, `Http2`, `UnencryptedHttp2`), Go's
`http.Protocols`; `UnencryptedHttp2` is HTTP/2 with prior knowledge on a
plain connection. `with_h2_config(H2Config)` sets Go's `HTTP2Config`
(streams, frame size, windows, HPACK tables, PING keepalive, write
timeout). Go backend only for now: the native backends store the set
and config until decision 0434's native step lands.

## 2026-10-30: feature: mutual TLS (with_client_auth, with_cert, verified_chains)

- Kind: feature
- Commit: this commit
- Found by: decision 0433 (gRPC over TLS and service meshes need mTLS)
- Go reference: crypto/tls Config.ClientAuth, ClientCAs, Certificates; ConnectionState.VerifiedChains (go1.26.1)
- Test: golden tls_mtls (each of the five modes against no certificate, another CA's, one without the clientAuth usage, and a good one)

`Server.with_client_auth(mode, roots)` asks a TLS client for its
certificate in one of Go's five modes and verifies it against `roots`
(the system's when None); `Client.with_cert(cert)` presents one when a
server asks; `TlsState.verified_chains` has the chains that verified the
client's certificate. The native TLS 1.3 server asks and verifies as Go
does (CertificateRequest with the CAs' names, the client's Certificate
and CertificateVerify, the clientAuth usage, Go's alerts); a native
client presents its certificate only when a named CA issued it
(Client.with_cert keeps the pool apart with a copy of the roots, the
system's too; TLS 1.2 clients and servers do client certificates since
the std/tls entry of 2026-10-30). Also on the native backends, `with_tls` keeps
the server's earlier settings (read timeouts, header and body limits)
instead of resetting them.

## 2026-10-30: feature: full duplex and pipe (with_full_duplex, pipe, PipeWriter)

- Kind: feature
- Commit: this commit
- Found by: decision 0433 (gRPC bidirectional streams)
- Go reference: net/http ResponseController.EnableFullDuplex, io.Pipe and PipeWriter.CloseWithError (go1.26.1)
- Test: golden http_pipe (a piped request body read in pieces by the server, a body cut with an error, a local reader's max and error), the same output on the Go and native backends

`Response.with_full_duplex()` lets a streamed handler read the request
body while it writes over HTTP/1.1 (HTTP/2 streams always may), as
`EnableFullDuplex`. `http.pipe()` returns a PipeWriter and the
BodyReader it fills (Go's io.Pipe): each write waits for the reader,
`close` ends the body, `close_with_error(e)` makes the reader's next
read `Err(e)`, and a write after close is `Io("io: read/write on closed
pipe")`. With `Request.with_body_reader` and `Client.open` one task
writes a request while another reads the response. The native HTTP/1.1
full duplex wire part (server respond, client open with a body sender
task; golden http_duplex) is in; the native HTTP/2 client's non-blocking
pipe pump is still to come; `pipe` works on every backend.

## 2026-10-30: feature: trailers (decision 0433)

- Kind: feature
- Commit: this commit
- Found by: the gRPC readiness survey of 2026-10-30, gap 1 (grpc-status and grpc-message are trailers)
- Go reference: net/http's TrailerPrefix, Header "Trailer", Request.Trailer, Response.Trailer; httpguts badTrailer; h2 writeChunk (trailers with no body: the response HEADERS, then the trailing HEADERS)
- Test: internal/llvm/testdata/golden/http_trailers (HTTP/1.1 and HTTP/2, native output equal to the Go backend's: want.txt equals want_go.txt)
- Timing: none (no hot path changes; a trailer is a header write after the body)

`BodyWriter.set_trailer`, `Response.with_trailer_names`,
`BodyReader.trailers`, `ClientStream.trailers` and
`Request.with_trailers(names, f)` send and read fields after a body, as
net/http does: names canonical and sorted, Go's forbidden names dropped,
a request's trailers declared in its head (Go's HTTP/2 server keeps only
declared ones), and an HTTP/1.1 streamed response with no body carrying
only the trailers it declared. The Go
backend maps them to net/http. The native wire (Xo Lang 3): the server
starts a chunked response whenever trailers are declared or set (HTTP/2:
a trailing HEADERS frame with END_STREAM), the client keeps a chunked
trailer section and an h2 trailing HEADERS, and a request's trailers go
out as the chunked trailer section or an h2 trailing HEADERS (a streamed
h2 body sends no content-length: 0). A server keeps a request's trailers
only when its Trailer field declared them; the parsed head carries that
field (Req.trailer, H2Req.trailer) since Go removes it from the header.
Names sort with `x = x.sort()` (List.sort returns a new list).

## 2026-10-30: fix: a header field folded over several lines read its first byte wrong on Linux

- Kind: fix
- Commit: this commit
- Found by: internal/llvm TestHTTP1ParseDiff on linux/arm64 (the lead's offload host list, 2026-10-29)
- Go reference: net/http's textproto.Reader.ReadMIMEHeader joins an obsolete line fold with one space
- Test: http1_test.xo "an obsolete line fold joins the lines with one space"; gotchas/raw-address-outlives-bytes.md

The request parser read a folded field's value through a raw address after
the joined text was released, so on Linux the allocator's free-list word
overwrote its first byte ("Accept:a\r\n b" gave "b b"); the text is kept
alive until the value is copied.

## 2026-10-29: perf: a streamed response body is read in large pieces straight into their bytes

- Kind: perf
- Commit: this commit
- Found by: bench/results/httpproxy-2026-10-29.md (the proxy 6x Go's time)
- Go reference: bufio.Reader.Read's fast path (a read of at least the
  buffer's size goes straight into the caller's slice), io.Copy's 32 KiB
  buffer; decision 0404
- Test: http_client_stream, http_reverse_proxy goldens; xo bench --ab in
  bench/results/httpproxy-direct-2026-10-29.md

ClientStream.read reads a piece of 16 KiB or more straight from the
connection into its own bytes when nothing is buffered (one system call, no
copy), and the reverse proxy reads 64 KiB pieces: 1 GiB through the proxy in
187 ms against 581 ms before, and 219 ms for Go's httputil.ReverseProxy.

## 2026-10-29: fix: tls_state no longer reads the TLS connection's state from the handler's task (a data race over HTTP/2)

- Kind: fix
- Commit: this commit
- Found by: internal/llvm TestGoldenTasks/http_tls under ThreadSanitizer, 3 of 3 runs on main after edd6fd640 (Xo Lang 2's bisect): tlssrv load in the handler's task against the connection's write on another thread
- Go reference: crypto/tls's ConnectionState is read under the connection's handshake mutex; Xo records it once instead
- Test: TestGoldenTasks/http_tls and http_tls_state (XO_THREADS 1, 2 and default), TestGoldenRun/http_tls and http_tls_state

`Request.tls_state` (edd6fd640) read the connection's TLS state with
tlssrv.negotiated, which loaded the Conn from the connection's block in
the request's task while an HTTP/2 connection's writer, on another
thread, was moving the same Conn out and back (take_conn, stash).
The handshake now records what it negotiated once, when it completes,
in a third word of the connection's block (an Any, as the server's
SrvTls), before any request's task exists; negotiated reads that word
and never the Conn, and free releases it. The values are the same.

## 2026-10-30: test: the http_tls golden expects url.scheme "https" over TLS (on purpose, unlike Go)

- Kind: test
- Commit: this commit
- Found by: main red on golden http_tls after ea6f13462 (want.txt stale since 53b878858 set the scheme over TLS)
- Go reference: net/http's server leaves URL.Scheme empty on a request it parses (NewServerRequest parses the request target alone); Xo differs on purpose
- Test: golden http_tls (both backends, arm64 and LLVM, Go parity), TestH2Diff

A request served over TLS has `url.scheme` "https" on both backends
(spec std.md, `tls()`: "url.scheme is then https"), whether it came by
HTTP/1.1 (53b878858) or HTTP/2 (ea6f13462); a plain request keeps "".
This differs from Go, whose server leaves URL.Scheme empty, so a handler
that forwards the request or builds a redirect does not have to consult
`tls()` itself. The golden's want.txt was re-recorded; no behavior
changed.

## 2026-10-30: feature: router not_found and method_not_allowed hooks, Allow on a 405, error_html

- Kind: feature
- Commit: this commit
- Found by: the HTTP server gaps survey (part 3): the 404 and 405 were fixed JSON, a 405 carried no Allow
- Go reference: net/http 1.22's ServeMux (Allow on a 405 lists the pattern methods of the path); chi's NotFound and MethodNotAllowed; html/template-free pages as Go's Error gives text
- Test: golden http_router_hooks (both backends)

`r.not_found(h)` and `r.method_not_allowed(h)` answer a miss in place
of the `{"error":"not found"}` and `{"error":"method not allowed"}`
JSON; a hook runs as a handler (inside the middleware, an error a
response, a fault a 500). A 405, default or a hook's without one,
carries `Allow` with the methods of the routes that match the path in
registration order without repeats. A mounted router's hooks are not
copied: its misses are the outer router's. `http.error_html(status,
title, detail)` is an HTML page (text/html, nosniff) with the title in
<title> and <h1> and the detail in <p>, both escaped.
## 2026-10-30: feature: Request.tls_state, what the connection's handshake negotiated

- Kind: feature
- Commit: this commit
- Found by: the HTTP server gaps survey (part 3; Go's r.TLS had only `tls()`)
- Go reference: net/http's Request.TLS (tls.ConnectionState: Version, CipherSuite, ServerName, DidResume, PeerCertificates)
- Test: golden http_tls_state (HTTP/1.1 and HTTP/2 over TLS, plain, a struct literal)

`req.tls_state()` is `Some(TlsState)` over TLS: the version ("TLS 1.2"
or "TLS 1.3"), the suite's name as Go's CipherSuiteName, the server name
the client asked for (SNI; "" for an IP address), whether the handshake
resumed a session, and the client's certificates (std/x509; empty: the
server asks the client for none, as Go's default ClientAuth). None over
plain HTTP and for a request built as a struct literal. Native: the
connection's tlssrv state is read once when the request is built
(tlssrv.negotiated, hidden field tls_neg); Go: from hr.TLS.

## 2026-10-30: fix: an HTTP/2 request's url.scheme is https when the :scheme pseudo header is missing

- Kind: fix
- Commit: this commit
- Found by: TestH2Diff on windows/amd64 (CI run 40926030751: native `scheme=` against Go's `scheme=https`)
- Go reference: net/http's HTTP/2 server fills URL.Scheme from :scheme
- Test: internal/llvm TestH2Diff (the Mac), golden http_server_request

stream_request (std/native/http.xo) takes "https" when h2srv's request
has no scheme; HTTP/2 is reached through with_tls only, so the scheme is
known. The HTTP/1.1 path got the same from the connection's TLS state in
53b878858.
## 2026-10-30: fix: the server's timeouts reach TLS connections (server gaps, part 3 item 1)

- Kind: fix
- Commit: this commit
- Found by: the lead's review of part 2 (a slow client held an HTTPS connection open for ever: the slowloris case)
- Go reference: net/http's ReadTimeout and ReadHeaderTimeout apply to a TLS conn's handshake and reads
- Test: golden http_server_limits (a client that connects to the HTTPS server and sends nothing is closed after the head timeout)

hconn.recv_until gives the TLS connection its deadline (tlssrv.set_deadline)
around each record read, and a timed out wait is the same -2 as a plain
socket's; the handshake, which hserve.hc_h2 runs before the connection's
limits are stored, gets net/http's handshake deadline (the smaller of the
read and head timeouts, http.xo handshake_deadline) and fails past it, so
the connection closes without a reply. Before, a TLS read waited without
a deadline; a first draft that set it only in recv_until left the
handshake unbounded, and the golden's silent client hung the recording
(gotchas/tls-handshake-before-limits.md).

## 2026-10-29: feature: server timeouts and limits (server gaps, part 2)

- Kind: feature
- Commit: this commit
- Found by: std-coverage.md rank 11 (the lead's list)
- Go reference: net/http Server.ReadTimeout, ReadHeaderTimeout, IdleTimeout, MaxHeaderBytes, MaxBytesReader (go1.26.1)
- Test: golden http_server_limits (a 431 over the header cap, a 413 over the body cap and through max_body, a late head closed without a reply), identical on both backends

`with_read_timeout`, `with_read_header_timeout`, `with_idle_timeout`,
`with_max_header_bytes` (431 over it) and `with_max_body` (413 before the
handler, the connection closed) on Server, and the `max_body(n)`
middleware (413 with Go's text). Natively the limits sit on each connection's parse record
(http1.Limits) and the reads take a deadline (hconn.recv_until); the Go
backend sets net/http's fields.

## 2026-10-29: feature: Request host, tls, raw_query, escaped_path; redirect, error_page, not_found (server gaps, part 1)

- Kind: feature
- Commit: this commit
- Found by: std-coverage.md rank 11 (the lead's list)
- Go reference: net/http Request.Host, Request.TLS, URL.RawQuery, URL.EscapedPath, Redirect, Error, NotFound (go1.26.1)
- Test: std/native/http_test.xo (Go's TestRedirect table, Error's text, escaped_path cases), internal/llvm golden http_server_request

`req.host()`, `req.tls()` (and `url.scheme` "https" over TLS, for a proxy's
X-Forwarded-Proto), `req.raw_query()`, `req.escaped_path()`; `http.redirect`
(Go's relative resolution and Location escaping), `http.error_page`,
`http.not_found`. Part 2: the server's timeouts, header and body limits.

## 2026-10-29: feature: WebSocket and other protocol switches through http.reverse_proxy

- Kind: feature
- Commit: this commit
- Found by: decision 0404 stage E (a gateway in front of a WebSocket service)
- Go reference: net/http/httputil reverseproxy.go handleUpgradeResponse,
  upgradeType; decision 0404 stage E
- Test: internal/llvm/testdata/golden/http_reverse_proxy_ws (native and Go)

ReverseProxy.serve keeps a requested `Upgrade` on the outgoing request and, on
the target's 101, hands the client's connection over and copies bytes both ways
until either side ends: a WebSocket echo, a 100 KB binary message, the
subprotocol and a server close pass through. A switch to another protocol than
the one asked for is 502. A client stream on a 101 now reads the connection's
raw bytes (llrt/http1.xo read_head).

## 2026-10-29: feature: a reverse proxy (http.reverse_proxy)

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md): an API gateway
- Go reference: net/http/httputil reverseproxy.go (ReverseProxy, Rewrite,
  ProxyRequest.SetURL and SetXForwarded, removeHopByHopHeaders,
  joinURLPath/singleJoiningSlash, flushInterval); decision 0404 stage D
- Test: internal/llvm/testdata/golden/http_reverse_proxy (native and Go)

`http.reverse_proxy(client, target)` and `ReverseProxy.serve(req, body)` for a
`handle_stream` route: path joined and query merged as SetURL does, hop-by-hop
fields dropped both ways, X-Forwarded-For/-Host/-Proto set, a `with_rewrite`
hook, both bodies streamed (decision 0404 stages B and C), redirects passed
through, event streams flushed, 502 and 504 on failures. Not yet: Upgrade
tunnels (stage E).

## 2026-10-29: feature: a request body sent as it is read (Request.with_body_reader)

- Kind: feature
- Commit: this commit
- Found by: decision 0404 stage C (a proxy or upload relay had to hold the
  whole request body)
- Go reference: net/http NewRequest with an io.Reader body (ContentLength
  -1, no GetBody: transfer.go chunked writing; client.go redirectBehavior
  returns a 307/308 without GetBody); decision 0404 stage C
- Test: internal/llvm/testdata/golden/http_client_body_reader (native and Go)

`Request.with_body_reader(b)` sends a BodyReader's bytes as they are read,
chunked unless the request gives Content-Length, over HTTP/1.1 and HTTP/2
(DATA frames as the window allows). The body is never sent twice: a 307 or
308 is returned, a 303 is followed as a GET without it.

## 2026-10-29: fix: std builds for separate compilation again after Client.open

- Kind: fix
- Commit: this commit
- Found by: xo-lang-6 (`xo dev stdobj` failed with XO0510 in std/native/http.xo)
- Go reference: none (an Xo checker rule, core 5.3)
- Test: `xo dev stdobj` in the gate; http_client_stream and http_tls goldens

ClientStream.read moved its gzip Decompressor (move only) inside its loop,
which the full check of std rejects; default builds fell back to the
whole-program build. The decoder now moves in a helper outside the loop.

## 2026-10-29: feature: an HTTP/2 response body read as it arrives

- Kind: feature
- Commit: this commit
- Found by: decision 0404 stage B2 (B1 buffered an HTTP/2 body whole)
- Go reference: net/http h2_bundle.go transportResponseBody.Read and
  Close (RST_STREAM CANCEL); decision 0404 stage B2
- Test: internal/llvm/testdata/golden/http_tls (stream big, stream early,
  after early; native and Go)

`Client.open` over https with h2 no longer reads the whole body first:
each `read` returns the next DATA frame's bytes, so memory stays flat as the
body grows. `close` before the end sends RST_STREAM CANCEL and keeps the
connection pooled for the next request.

## 2026-10-29: feature: a response body read as it arrives (Client.open)

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md): a download, a
  proxy or an event stream had to hold the whole body
- Go reference: net/http Response.Body (transfer.go body, chunked.go
  chunkedReader), Transport's gzip reader; decision 0404 stage B1
- Test: internal/llvm/testdata/golden/http_client_stream (native and Go)

`Client.open(req)` returns a `ClientStream` once the head has arrived:
`head()`, `read(max)`, `read_all(max_size)` and `close()`. A Content-Length,
chunked or until-close body is read in pieces natively (a body cut short is
`Protocol`, `unexpected EOF`), gzip is decoded as it is read, and
`Request.with_idle_timeout(d)` limits each read while the client's timeout
bounds only the head. A body read to its end returns its connection to the
pool; one closed or dropped early closes it. `send` is unchanged. Not yet:
an HTTP/2 response's body is still buffered whole before `open` returns
(stage B2), so memory grows with the body over https with h2.

## 2026-10-29: feature: the client's redirect policy and a request's own time limit

- Kind: feature
- Commit: this commit
- Found by: the 2026-10-29 std survey (std-coverage.md): an API gateway could
  not pass a 3xx through
- Go reference: net/http client.go Client.CheckRedirect (ErrUseLastResponse,
  defaultCheckRedirect), Client.Timeout; decision 0404 stage A
- Test: internal/llvm/testdata/golden/http_client_policy (native and Go)

`Client.with_redirects(max)` follows at most max redirects: 0 returns the 3xx
as it came with its Location untouched (a proxy passes it through), and past
max the request fails with `Protocol` (`stopped after MAX redirects`). The
default stays 10 and now follows 10 (the spec's "up to 10"; Go's
defaultCheckRedirect counts requests and followed 9 here before).
`Request.with_timeout(d)` gives one request its own limit for the whole
exchange in place of its client's. A redirect to another host already
dropped Authorization and cookies; spec 4.5 now says so.

## 2026-10-26: fix: an Expires date outside Time's range has no Expires on the Go backend

- Kind: fix
- Commit: this commit
- Found by: the corpus's range rows on the Go backend (a four digit year
  outside 1678 to 2262)
- Go reference: net/http cookie.go ParseSetCookie, whose time.Time holds any
  year up to 9999; Xo's Time is int64 nanoseconds (decision 0364)
- Test: internal/stdtest/testdata/corpus/http (SET_COOKIE_LINES, go, arm64
  and LLVM)

`xoCookie` (std/rt/cookie.go) converted net/http's Expires with UnixNano,
which wraps a date outside Time's range to a different year, so the Go
backend reported an Expires for "Wed, 16 Dec 9999 09:51:07 GMT" where
native's `cookie_date` has none (time.instant refuses it). A date outside
the range now leaves Expires unset on both. net/http still holds the date,
so the corpus's oracle drops it for Xo. The range's ends are held: "Fri, 01
Jan 1678 00:00:00 GMT" and "Thu, 10 Apr 2262 00:00:00 GMT" both read.

## 2026-10-26: fix: a cookie date's month is read in any case, as Go reads it

- Kind: fix
- Commit: this commit
- Found by: the seeded cookie table (internal/stdtest/testdata/corpus/http) on
  go, arm64 and LLVM
- Go reference: net/http cookie.go readSetCookies through time.Parse's month
  lookup (format.go lookup and match: ASCII case folding, the three letter
  names only); go1.26.1
- Test: internal/stdtest/testdata/corpus/http (COOKIE_LINES, SET_COOKIE_LINES
  and the seeded rows, go, arm64 and LLVM)

`cookie_date` compared the month field with "Jan" to "Dec" exactly, so
"Sun, 29 JUN 2021 22:46:55 GMT" kept no Expires where Go's parser reads June
(Go compares the first three bytes under ASCII case folding). The comparison
now folds case, which matches Go for every spelling: "JUN", "jun" and "juN"
read, and "June" still does not, since Go's lookup matches "Jun" and leaves
the "e" for the layout's next space to refuse. The corpus's cookie tables are
now seeded as well: 2020 Cookie values and 2050 Set-Cookie values built from
every separator, bare token, quoted and escaped value, control byte, and an
Expires in all three of Go's date shapes with every weekday, month and zone
spelling.

## 2026-10-26: fix: a header name with a non-token byte is matched exactly

- Kind: fix
- Commit: this commit
- Found by: the seeded header corpus (internal/stdtest/testdata/corpus/http)
  on go, arm64 and LLVM
- Go reference: net/http Header.Get through textproto.CanonicalMIMEHeaderKey
  (a name with a byte that is not a token is canonicalized to itself, so its
  lookup is exact); go1.26.1
- Test: internal/stdtest/testdata/corpus/http (HEADERS and REPEAT_HEADERS,
  plus "a struct literal's headers read the way the spec says")

`header` (and `header_values`, which reads the same way) canonicalizes the
lookup name and, when that key is absent, still falls back to a
case-insensitive scan of the stored names, which is the documented behavior
(std.md 4.3) and what a struct literal's `headers: {"content-type": ...}`
relies on. The fallback is now limited to lookup names that are RFC tokens:
a name with a byte outside httpguts.ValidHeaderFieldName's set matches the
canonical key only, as Go's Header.Get reads it, since Go's canonical name
for such a name is the name itself. So with the field `"\tType"` stored,
`header("\tTYPE")` is None as in Go, while `header("CONTENT-TYPE")` still
reads a field stored as `content-type`. The HTTP/1.1 and HTTP/2 parsers
canonicalize a received name already, so they read the same.

## 2026-10-26: fix: a cookie date's weekday and zone are checked as Go checks them

- Kind: fix
- Commit: this commit
- Found by: the cookie corpus's follow-up rows, xo-lang-4's review of the
  Expires fix below
- Go reference: net/http cookie.go readSetCookies with the cookie TimeFormat,
  through time.Parse's weekday and zone parsing; cookie_zone_len is a port of
  parseTimeZone, parseGMT and parseSignedOffset (go1.26.1
  src/time/format.go, over bytes as Go does)
- Test: internal/stdtest/testdata/corpus/http (SET_COOKIE_LINES, go, arm64 and
  LLVM)

`cookie_date` read the weekday without looking at it and the zone without
looking at it either, so "Xyz, 09 Jun 2021 10:18:14 GMT" and
"Wed, 09 Jun 2021 10:18:14 gmt" both became expiry dates. Go leaves Expires
unset for both, and so does Xo now. The weekday is one of the seven three
letter names in any case ("wed" and "WED" read). The zone is what Go's parser
reads in full, ported from the standard library: three to five capitals where
a fourth or fifth must end in T ("UTC", "CEST", "AKDST"), a fourth may also be
"WITA", "ChST" and "MeST" are the only zones with a lower case letter, "GMT"
takes a sign and an hour of 23 or less ("GMT+8", "GMT-23"), and a bare sign is
an hour of 23 or less with nothing after it ("+05", "+0023"). A lowercase
"gmt", a numeric "123", a bare sign in front of four digits ("+0100", 100
hours), "GMT+24", "UTC+8", "ABCD", and a non ASCII zone all leave Expires
unset, exactly as in Go. The time itself is still read as UTC.

## 2026-10-26: fix: an Expires date in the standard cookie format is no longer dropped

- Kind: fix
- Commit: this commit
- Found by: the new cookie corpus (internal/stdtest/testdata/corpus/http) on
  arm64 and LLVM; the Go backend never had the bug
- Go reference: net/http cookie.go readSetCookies, which parses Expires with
  the cookie TimeFormat (go1.26.1)
- Test: internal/stdtest/testdata/corpus/http (SET_COOKIE_LINES, go, arm64 and
  LLVM)

`parse_set_cookie` read an Expires value through cookie_date, which split the
date on spaces and accepted four fields after the weekday or three. Go's cookie
TimeFormat leaves five ("02 Jan 2006 15:04:05 GMT"), so every standard
Set-Cookie date returned None and the cookie lost its expiry: a value handed to
a browser again kept a session cookie that should have expired. The hyphenated
form with a four digit year ("02-Jan-2006 15:04:05 GMT") still reads, as it does
in Go, whose parser is lenient about the separator; a two digit year (RFC 850), a
date with no zone, and the ANSIC form leave Expires unset, which is also what Go
does.

## 2026-10-23: fix: http.new_request's path on the Go backend

- Kind: fix
- Commit: this commit
- Found by: TestStdCorpora on the Go backend (added the same day; before it a -run with /go/ matched nothing)
- Go reference: net/url URL.Path and URL.EscapedPath
- Test: internal/stdtest/testdata/corpus/httpurl on the Go backend

`http.new_request` set `req.path` to the loose parse's path on the Go
backend: "/" for a URL with no path (`http://www.google.com`), the escaped
form for a path with escapes (`/a%20b`). The native backends, and Go, give
"" for no path (and for an opaque URL), the decoded path otherwise, and
`%XX` only for bytes that are not UTF-8 (`/é/%FF`). The Go backend now
does the same. `req.url.path` was already right.

## 2026-10-23: behavior: HttpErr, ClientErr, UrlErr, CookieErr and TlsErr print plain messages (decision 0358)

- Kind: behavior
- Commit: this commit
- Found by: the Intel field report on 0.26.0 (error texts nested Debug output)
- Go reference: net/http, net/smtp and text/template error texts
- Test: golden error_text_http (arm64, LLVM, Go parity)

`Display` printed the Debug form (`Connect(msg: "...")`, `UrlErr(msg: ...)`).
It prints plain text now: `HttpErr` the message of an `Io` and `canceled`;
`ClientErr` `invalid URL: MSG`, MSG for a `Connect`, `request timed out`,
`protocol error: MSG`, `canceled`; `UrlErr`, `CookieErr` and `TlsErr` their
message. `Debug` and value comparison are unchanged; a program that
matches the old text must change.

## 2026-10-23: test: parse_url and new_request against Go's net/url

- Kind: test
- Commit: this commit
- Found by: Go's url_test.go inputs and 4,000 seeded random URLs and 1,200 queries run through url.Parse and ParseQuery
- Go reference: net/url url_test.go (Copyright 2009 The Go Authors, BSD-style license), url.go (go1.26.1)
- Test: internal/stdtest/testdata/corpus/httpurl (TestStdCorpora, arm64 and LLVM)

The corpus compares parse_url (scheme, host, port, path, first value of each
query key, and its three error texts) and new_request (path, url path, query)
with Go. It found the postgres multi-host gap fixed in std/url.

## 2026-10-23: perf: query_parts reads a plain query in place

- Kind: perf
- Commit: this commit
- Found by: XO_PROF=cpu of http.new_request (query_parts was 18% of it)
- Go reference: net/url url.go parseQuery (go1.26.1)
- Test: std/native/http_test.xo "a plain query is read in place", internal/stdtest/testdata/corpus/url (TestStdCorpora), goldens http_form and http_router_paths

`http1.query_parts` counted the `&` bytes one at a time, searched each pair
with a string find, and ran every key and value through `neturl.unescape`
(a byte loop and a UTF-8 check each). It now finds `&` with
`scan.find_byte`, checks for a `;` once, and when the whole query has no
`%` or `+` and is valid UTF-8 (three word-at-a-time scans) every key and
value is a view of the query. `xo bench --ab`, 300k `new_request` calls on
four URLs with plain queries: 0.756 instructions and 0.821 wall against the
old code (156.6 to 128.7 ms), and against Go's `http.NewRequest` with
`URL.Query()` 0.827 instructions, wall within noise, peak memory 0.17;
the mixed row of 400k calls (two queries, one with `%20` and one with
`+`): 0.937 instructions, 0.98 wall, Go 0.722 instructions, 0.704 wall.
bench/results/query-parts-2026-10-23.md.

## 2026-10-23: perf: new_request and parse_url parse the URL once, and url_parse scans a word at a time

- Kind: perf
- Commit: this commit
- Found by: XO_PROF=cpu of http.new_request (url_parts was 47% of it, called twice)
- Go reference: net/url url.go Parse (go1.26.1); http.NewRequest
- Test: internal/stdtest/testdata/corpus/url (TestStdCorpora), golden http_router

`new_request` and `parse_url` called `http1.url_parts` twice for one URL;
they now share one result. `neturl.url_parse` finds the control-byte check
with `scan.ctl_at`, the query with `scan.find_byte`, and the path's escape
with `scan.find_byte` instead of per-byte loops, and the raw query is a
view. `xo bench --ab` of 400k `new_request` calls on four URLs: 2.94 G to
2.06 G instructions (0.699) and 164 ms to 114 ms for the parse-once change,
then 2.06 G to 1.83 G (0.890, wall 0.882) for the scans. Against Go's
`http.NewRequest` the same row is 0.779 of its instructions and 0.838 of
its wall time (bench/results/url-parse-2026-10-23.md).

## 2026-10-23: perf: a router's path parameters skip the range clamp

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 3's router dispatch row (three routes, 200,000 dispatches) against Go's ServeMux
- Go reference: net/http ServeMux.PathValue (a substring of the path)
- Test: none (same output; the dispatch row prints the same checksum before and after)

`bind_in` binds a parameter with `raw.str_cut` (the bounds are the segment
bounds already checked) instead of `raw.str_slice`, which clamped both ends
again. Dispatch of a prepared request: 905 M to 864 M instructions,
`xo bench --ab` instructions 0.954 (interval 0.954 to 0.955), wall within
noise (0.968, n=10).

## 2026-10-22: perf: the HTTP date layout is parsed once

- Kind: perf
- Commit: 8ddf1bbe0
- Found by: bench/results/time-format-cache-variants-2026-10-22.md (the layout parse was 62% of a text-layout format)
- Go reference: none (Go's time.Format has no parsed layout type; this is the lazy const of decision 0332)
- Test: golden http_cookies (the Expires text), http_serve_content (Last-Modified)

`Cookie.display`'s Expires and the Last-Modified header format their date with
a lazy `const HTTP_DATE_LAYOUT: time.Layout` instead of parsing the layout
text on every call. Set-Cookie with an Expires over 300,000 cookies: 3,079 M
to 1,320 M instructions, `xo bench --ab` wall 0.427x (interval 0.403 to 0.465),
the same output.

## 2026-10-22: perf: a server's connection fibers no longer hold 2 MiB each of resident memory

- Kind: perf
- Commit: d08537802
- Found by: bench/perf/netgo comparison (CRUD server peak RSS at 1024 connections: Xo 2.1 to 2.3 GB, Go 66 to 78 MB)
- Go reference: none (goroutine stacks start small)
- Test: none (a memory figure of the Linux runtime: /proc/PID/smaps, gotchas/fiber-stacks-huge-pages.md)

Each fiber has a 64 MiB reserved stack. On a Linux with transparent huge
pages set to `always` (Docker Desktop's VM and the default of several
distributions) the first touch of a stack's top faulted in a whole 2 MiB
page, so every live connection cost 2 MiB of resident memory. The runtime
now marks fiber stacks MADV_NOHUGEPAGE: the CRUD server at 1024
connections holds 47 to 57 MB (64 connections: 21 MB against 158 MB), and
its throughput did not change within the run to run noise.

## 2026-10-21: perf: a large streamed write goes out as one chunk

- Kind: perf
- Commit: f0aae00e5
- Found by: bench/perf/tlsrate (TLS bulk over HTTP/1.1: Xo 220 MB/s against Go's 2,000 MB/s on the same TLS code; HTTP/2 downloads were at parity)
- Go reference: net/http response.Write through bufio.Writer.Write (a write larger than the buffer's room goes around an empty buffer)
- Test: internal/llvm/testdata/golden/http_stream (a 3000 byte write: one chunk bb8, not 800 and 3b8)

BodyWriter.write was bufio's WriteString: it filled a 2 KiB buffer and
sent a chunk (head, data, CRLF: three TLS records over TLS) per 2 KiB, so
a body written in 64 KiB pieces made 32 chunks each. It now follows
Write: a write larger than the room left goes out whole when the buffer
is empty (HTTP/1.1 one chunk, HTTP/2 DATA frames of the same data).

## 2026-10-21: fix: a streamed body's read error sticks

- Kind: fix
- Commit: b5d9c4c66
- Found by: review after the websocket close fix (the same Mutex.with pattern)
- Go reference: net/http body (a read error is returned again)
- Test: internal/llvm/testdata/golden/http_stream_body (bad chunk), http_multipart_stream

On the native backends, `BodyReader.read` after a failed read, and
`PartReader.next` or `read` after a failed one, read the connection
again instead of returning the same error.

## 2026-10-21: feature: BodyReader.multipart (multipart parts from the stream)

- Kind: feature
- Commit: 942699fbf
- Found by: decision 0310 phase 3
- Go reference: net/http Request.MultipartReader; mime/multipart Reader.NextPart and Part.Read; mime/quotedprintable (go1.26.1)
- Test: internal/llvm/testdata/golden/http_multipart_stream (native identical to the Go backend)

A `handle_stream` route can read a `multipart/form-data` body part by
part as it arrives: `b.multipart(req)` returns a `PartReader` whose
`next()` gives each part's name, file name, and headers
(`header_values` for repeated ones) and whose `read(max)` gives the
part's data, so an upload is never held in memory whole.

## 2026-10-21: feature: streamed request bodies over HTTP/2

- Kind: feature
- Commit: 8ac517e64
- Found by: decision 0310 phase 2
- Go reference: golang.org/x/net/http2 server (requestBody, the stream's inflow returned as the handler reads, RST_STREAM NO_ERROR for an unread body; go1.26.1's bundled h2_bundle.go)
- Test: internal/llvm/testdata/golden/http_stream_body_h2 (300,000 byte bodies over a 64 KiB stream window, small reads, a body read halfway and one not read; native identical to the Go backend)

A `handle_stream` route on an HTTPS server now reads an HTTP/2 request
body as its DATA frames arrive: the native server hands the request
over at its HEADERS and gives the stream's window back as the handler
reads, as net/http's.

## 2026-10-21: feature: Router.handle_stream (streamed request bodies)

- Kind: feature
- Commit: f2358842
- Found by: owner's request (2026-10-21), decision 0310
- Go reference: net/http's Request.Body (body, chunkedReader, expectContinueReader, chunkWriter.writeHeader's discard; go1.26.1)
- Test: internal/llvm/testdata/golden/http_stream_body (17 exchanges: Content-Length and chunked bodies read 3 bytes at a time, in parts as they arrive, read_all, unread bodies small and large, 100-continue read and unread, malformed chunks; native identical to the Go backend)

`r.handle_stream(pattern, fn(req, body) { ... })` calls the handler
before the body is read; `body.read(max)` and `body.read_all()` read it
as it arrives, as Go's `r.Body`. The native HTTP/1.1 server reads the
connection's input on demand, sends 100 Continue on the first read, and
after the handler discards an unread rest under 256 KiB or closes.

## 2026-10-21: feature: http.gzip_responses

- Kind: feature
- Commit: 4cddcc7b
- Found by: decision 0305 (approved by the owner, 2026-10-21)
- Go reference: none in Go's standard library; the rules of klauspost/compress gzhttp
- Test: internal/llvm/testdata/golden/http_gzip_responses

`r.wrap(http.gzip_responses())` compresses responses with gzip when the
request accepts it. It skips small bodies, compressed content types,
206s, and responses that already have a Content-Encoding. It adds
`Vary: Accept-Encoding`, weakens ETags, and compresses streamed bodies
through a Compressor that each flush reaches.
## 2026-10-21: feature: http.serve_content

- Kind: feature
- Commit: 5ff295e6
- Found by: owner's request (2026-10-21)
- Go reference: net/http ServeContent, serveContent, checkPreconditions, scanETag, parseRange (go1.26.1)
- Test: internal/llvm/testdata/golden/http_serve_content (35 requests: ranges, conditional requests, HEAD, content types; the native backends match the Go backend's net/http byte for byte, the multipart boundary aside)

`http.serve_content(req, name, modtime, content, headers)` returns the
response Go's ServeContent writes: Range requests (one range as 206
with Content-Range, several as multipart/byteranges), If-Match,
If-Unmodified-Since, If-None-Match, If-Modified-Since and If-Range
against modtime and the ETag in `headers`, 416 for a range outside the
content, and the Content-Type from `headers`, the name's extension, or
the content (decision 0306).

## 2026-10-21: feature: Request.client_ip behind trusted proxies

- Kind: feature
- Commit: b37efc91
- Found by: survey (client addresses behind load balancers; with Xo Lang 3)
- Go reference: net/netip (Prefix.Contains, Addr.Unmap); RFC 7239; the walk of chi's and gorilla's proxy headers middleware
- Test: internal/llvm/testdata/golden/http_client_ip (every backend)

`client_ip(trusted, header)`: the peer, or the rightmost address of the
named proxy header outside the trusted prefixes, with only that header
read and the first untrusted hop ending the walk (None when it is hidden,
unknown, or does not parse; reviewed with Xo Lang 3).

## 2026-10-21: feature: the client keeps HTTP/1.1 connections alive

- Kind: feature
- Commit: 04ea3e47
- Found by: per-line profile of bench/perf/httpclient (Xo Lang 3)
- Go reference: net/http Transport (getConn, tryPutIdleConn, persistConn.readLoop, shouldClose, isReplayable, shouldRetryRequest; go1.26.1)
- Test: internal/llvm/testdata/golden/http_client_keepalive (connections a raw server accepted, request by request, as the Go backend's); TestHTTP1ClientDiff (457 responses, llvm, go, arm64)

Natively, an HTTP/1.1 connection (plain or TLS) goes back to a pool
after a response read in full that leaves it open, and the next request
to the same scheme, address and server name takes it: at most 2 idle
per host, 100 in all, 90 s idle, as Transport's defaults. A response
with Connection: close, an HTTP/1.0 one without keep-alive, a body that
ends at EOF, or a 101 closes it; a connection the server closed while
idle is dropped when taken, and a replayable request that met a dead
pooled connection is sent once more on a new one. The client bench's
HTTP/1.1 workloads against a keep-alive server: 1,119M to 520M
instructions for 3000 requests (decision 0303).

## 2026-10-21: fix: the native client reads responses as net/http does

- Kind: fix
- Commit: c9dad995
- Found by: differential test against Go
- Go reference: net/http ReadResponse, readTransfer (parseTransferEncoding, fixLength, fixTrailer, shouldClose), internal/chunked.go; net/textproto readMIMEHeader (go1.26.1)
- Test: internal/llvm/testdata/golden/http_client_responses (349 raw responses: net/http's parsing cases and 300 random header blocks; arm64, LLVM, and the Go backend agree); internal/llvm TestHTTP1ClientDiff's 457 random responses now agree with the Go backend on every one

The native HTTP/1.1 client was lenient where Go's is strict, and
reported other texts. It took a bad or negative Content-Length, two
different ones, or one like `2, 2`; a Transfer-Encoding other than
chunked; a field line without a colon, with an empty or invalid name,
or with a control byte; and kept only the first line of a folded field.
Each is now Go's error (`net/http: HTTP/1.x transport connection
broken: bad Content-Length "x"`, `unsupported transfer encoding:
"gzip"`, `malformed MIME header line: ...`), a folded field is joined
with a space, equal Content-Length fields read as one, a closing
HTTP/1.1 response has no `Connection` field (as net/http removes it),
and the body's errors (`unexpected EOF`, `invalid byte in chunk length`)
come without the request, as Go's body reads give them. An error text
keeps a zero byte of the line it quotes (it was cut there), a status
line's bytes are quoted as Go's %q (`"\xff"`), and a Location that
url.Parse rejects is Client.Do's `Get "URL": failed to parse Location
header ...`. On the Go backend, an error text quoting a field line with
bytes that are not UTF-8 now has U+FFFD for them (decision 0259), as
natively. TestHTTP1ClientDiff now holds every backend to the Go
backend's output on all 457 of its responses (testdata/http1_client.txt
is the Go backend's).

## 2026-10-21: speedup: Router matching without a string per path segment

- Kind: speedup
- Commit: 73612e39
- Found by: per-line callgrind of bench/perf/http (Xo Lang 3)
- Go reference: none
- Test: internal/llvm/testdata/golden/http_router_paths (124 method and path cases, the same answers as before the change)

Natively, a request path without escapes is matched against the routes
where it is: its segments' offsets are found once and each route's
literals compared in place, so no segment strings or unescaped copies
are made (a path with escapes is matched as before). The plain HTTP
server's router code took 177 fewer instructions a request (callgrind,
arm64).

## 2026-10-21: fix: a request's repeated Set-Cookie fields

- Kind: fix
- Commit: a6a1e92a
- Found by: differential test against Go
- Go reference: net/http request parsing (readRequest, textproto ReadMIMEHeader) as the Go backend's fieldView shows it (go1.26.1)
- Test: internal/llvm/testdata/golden/http_header_fields (a request with two Set-Cookie fields); 467 raw requests (net/textproto and net/http parsing cases and 400 random header blocks) gave the same status, header Map, and body natively as on the Go backend

The native servers (HTTP/1.1 and HTTP/2) joined a request's repeated
Set-Cookie fields with ", " in `headers`; decision 0288 keeps the first,
as the Go backend and the client do (`header_values` gives each).

## 2026-10-21: fix: parse_url's host and fragment checks and error texts

- Kind: fix
- Commit: b2ef88de
- Found by: differential test against Go
- Go reference: net/url Parse, parseHost, unescape (encodeHost, encodeZone), setFragment (go1.26.1)
- Test: std/native/http_test.xo (1,586 URLs: url_test.go's inputs and 1,500 seeded random ones, with the Go backend's results)

Natively, `parse_url` accepted 11 URLs Go rejects (a bad escape in the
fragment, which Go's setFragment checks) and rejected an IPv6 literal
with an escaped zone Go accepts (`[fe80::1%25%65%6e%30]`, RFC 6874), and
every bad host was `invalid host "..."` instead of Go's reason (`invalid
port ":0x" after host`, `missing ']' in host`, `invalid character " "
in host name`, netip's text for a bad IPv6 literal). The native host
parser is now a port of Go's parseHost; the same texts reach an x509
certificate's URI name errors.

## 2026-10-20: feature: header fields with repeated names; fix: Set-Cookie

- Kind: feature
- Commit: 33fec016
- Found by: survey
- Go reference: net/http Header.Add, Header.Values, SetCookie, Response.Cookies (go1.26.1)
- Test: internal/llvm/testdata/golden/http_header_fields (HTTP/1.1 and HTTP/2; arm64, LLVM, and the Go backend agree)

`header_values` and `add_header` on Request and Response, and
`Response.with_cookie` and `cookies` (decision 0288, part 2): several
fields of one name are sent and received each. A fix with it: a client
response with several `Set-Cookie` fields was one value joined with ", "
(which `Expires` dates also contain); `headers` now shows the first, and
`header_values` or `cookies()` give all. The HTTP/2 client also kept the
last of a repeated field in the Map instead of joining them.

## 2026-10-20: feature: cookies

- Kind: feature
- Commit: 54da082a
- Found by: survey
- Go reference: net/http/cookie.go Cookie.String, ParseCookie, ParseSetCookie, readCookies, Request.Cookie, Cookies, AddCookie (go1.26.1)
- Test: internal/llvm/testdata/golden/http_cookies (the inputs of Go's cookie_test.go tables; arm64, LLVM, and the Go backend's net/http agree); std/native/http_test.xo (Go's TestCookieSanitizeValue, TestCookieSanitizePath, writeSetCookiesTests' domains and names)

`http.Cookie` with `display()` (the Set-Cookie value), `parse_cookie`,
`parse_set_cookie`, and `Request.cookie`, `cookies`, and `with_cookie`
(decision 0288, part 1). Bytes and domains Go drops with a log line are
dropped silently.
## 2026-10-20: change: form() parses the Content-Type as Go does

- Kind: change
- Commit: 11afca47
- Found by: review (Xo Lang 3)
- Go reference: net/http/request.go parsePostForm, TestParseFormUnknownContentType (go1.26.1)
- Test: std/native/http_test.xo (go TestParseFormUnknownContentType); internal/llvm/testdata/golden/http_form

`req.form()` reads the media type with `mime.parse_media_type`, so a
malformed Content-Type (`text/plain; boundary=`) is now an error with
mime's text, where it was an empty form; a well formed one behaves as
before.

## 2026-10-20: port: Request.multipart_form

- Kind: port
- Commit: 4de48c2e
- Found by: survey
- Go reference: net/http/request.go ParseMultipartForm, multipartReader (go1.26.1)
- Test: internal/llvm/testdata/golden/multipart_basic

`req.multipart_form()` reads a `multipart/form-data` body as a
`multipart.Form` with Go's Content-Type checks and error texts (decision
0282).

## 2026-10-20: fix: the client sends a URL's query as given

- Kind: fix
- Commit: 361a427c
- Found by: survey
- Go reference: net/http/client.go, net/url URL.RawQuery (go1.26.1)
- Test: internal/llvm/testdata/golden/http_form (repeated keys and their order); internal/llvm/testdata/golden/http_tls (HTTP/2)

The client rebuilt a request's query from `url.query`, a map holding one
value per key, so `client.get("http://h/s?q=a&q=b")` sent only `q=a`. It
now sends the query as given unless `url.query` was changed.

## 2026-10-20: feature: Request.form and Request.query_all

- Kind: feature
- Commit: e2be349f
- Found by: survey
- Go reference: net/http/request.go ParseForm, PostForm; net/url URL.Query (go1.26.1)
- Test: std/native/http_test.xo (Go's TestParseFormQuery, TestParseFormQueryMethods, TestParseFormUnknownContentType, TestParseFormSemicolonSeparator); internal/llvm/testdata/golden/http_form

`req.form()` parses an `application/x-www-form-urlencoded` body into
`url.Values`, and `req.query_all(name)` returns every value of a repeated
query key; the server keeps each request's raw query for it, over HTTP/1.1
and HTTP/2.
