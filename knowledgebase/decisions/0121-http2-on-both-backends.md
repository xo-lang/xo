# 0121: HTTP/2 over TLS on every backend

Status: Accepted (2026-10-10), implemented. Supersedes point 2 of 0106
("HTTP/1.1 only, also over TLS"), as 0106's owner confirmation
anticipated. Implemented in `std/rt/http_net.go` (Go backend),
`std/llrt/xo_h2.c` (native server and client), hooks in
`std/llrt/xo_http.c` and `xo_tls.c` (ALPN), `std/native/http.xo` and
`rthttp.xo`. Spec: std.md 4.6. Tests: TestH2Diff and TestH2Sanitizers
(`internal/llvm/h2_diff_test.go`), the h2 part of
`std/llrt/testdata/http_fuzz.c` (TestHTTPFuzz), TestTLSDiff and the
`http_tls` golden (now over HTTP/2). Numbers:
`bench/results/http2-2026-10-10.md`.

## Context

Decision 0106 turned HTTP/2 off on the Go backend because the native
backends had none: a program had to behave the same on every backend,
and an Xo server sees different requests over HTTP/2
(`User-Agent: Go-http-client/2.0`, cookies joined with `; `, no
`Connection`). The owner queued HTTP/2 for both. The native runtime has
no third party HTTP/2 library and takes no new dependency; Mbed TLS
(decision 0105) gives ALPN only.

## Decision

1. **When.** Over TLS only, when ALPN agrees on `h2`: a `with_tls`
   server offers `h2` then `http/1.1` (and prefers `h2`, as net/http
   does), the client offers the same for `https` URLs. Cleartext stays
   HTTP/1.1 on every backend: no h2c, neither by prior knowledge nor by
   `Upgrade` (net/http's default; TestServeNoH2C). The Go backend sets
   `http.Protocols` to HTTP/1 and HTTP/2 on its TLS server and its
   clients' transports.
2. **No new API.** `Request`, `Response`, handlers, middleware,
   `http.stream`, and `ClientErr` stay as they are; the protocol is not
   a field (a `proto` would let programs depend on the transport, and
   in process dispatch and `FakeNet` have none). What a program can see
   of HTTP/2 is what net/http shows: the request fields of Go's HTTP/2
   client, no `Connection` header, and HTTP/2's error texts (std.md
   4.6).
3. **The native server copies net/http's** (Go 1.26, `h2_bundle.go`):
   SETTINGS of MAX_FRAME_SIZE 1 MiB, 250 concurrent streams,
   MAX_HEADER_LIST_SIZE 1048896, a 4096 byte HPACK table, and 1 MiB
   stream and connection windows; the frame checks of `Framer.ReadFrame`
   and the stream state checks of `serverConn.processFrame` with the same
   connection errors (GOAWAY with the code, the last stream id including
   a frame that parsed) and stream errors (RST_STREAM); 431 for a header
   list over the limit, 400 with net/http's text for connection specific
   fields and `TE` other than `trailers`, `OPTIONS *` answered by the
   server; responses as `responseWriterState.writeChunk` writes them
   (`:status`, the handler's fields sorted and lower case, `Connection`
   dropped, a sniffed `content-type`, `content-length` when known,
   `date`), a streamed body through a 4 KiB buffer; `Connection: close`
   from a handler starts a graceful shutdown of the connection;
   RST_STREAM NO_ERROR after a response whose request body is still
   open; a fault in a streamed body resets the stream with
   INTERNAL_ERROR after what was already queued (net/http's
   `handlerPanicRST`), and the connection's other streams go on.
4. **Streams run as tasks.** The connection's task reads and writes all
   frames (one fiber: an Mbed TLS context may not be used by two threads
   at once); each complete request (its body read, as std/rt reads it
   before the handler) is a task in a scope of the connection's task,
   so handlers run concurrently as net/http runs a goroutine per stream.
   At most 250 handlers run per connection; 1000 more requests may wait;
   beyond that the connection ends with ENHANCE_YOUR_CALM (net/http's
   rapid reset limit, CVE-2023-44487). A stream task queues its response
   and wakes the connection's fiber, which encodes and sends it within
   the peer's windows, 64 KiB per stream in turn; a streamed body waits
   while more than 256 KiB of it is unsent.
5. **Flow control and limits.** Received bytes give their window back as
   they arrive (batched as net/http's `inflow`: from 4 KiB or when the
   peer's window would run out); a request body over 64 MiB
   (`XO_HTTP_MAX_BODY`) reaches the handler cut there and the rest is
   dropped. HPACK: header blocks are decoded as their frames arrive (a
   field cut by a frame waits), strings at most 1048896 bytes, a table
   size update only first and at most 4096, invalid Huffman padding and
   EOS rejected (COMPRESSION_ERROR); a CONTINUATION more than twice what
   the header list may still hold ends the connection (CVE-2024-27316,
   as `readMetaFrame`). At most 100 settings per frame, no duplicates.
6. **Stopping.** A stop request (core 7.5) or the end of `serve` sends
   GOAWAY NO_ERROR; streams in flight finish, then the connection
   closes. After a connection error the server reads and drops input
   for at most a second (net/http's `goAwayTimeout`) and closes. The
   native server logs nothing about a client's frames; the Go backend's
   `ErrorLog` filter drops net/http's `http2:` lines as it drops failed
   handshakes (0106).
7. **The native client** offers `h2` first and, as net/http's
   `Transport`, keeps the connection for the next request to the same
   address and server name with the same roots (up to 100 idle
   connections, 90 s each), so stream ids and error texts match
   (`stream error: stream ID 15; INTERNAL_ERROR; received from peer`).
   It sends one request at a time on a connection (requests at once open
   more connections), the header fields of `httpcommon.EncodeHeaders`
   (`user-agent: Go-http-client/2.0`, `accept-encoding: gzip`,
   `content-length`, a field per cookie), rejects connection specific
   request fields with Go's error, sends a request again on a new
   connection when a reused one did not take it (closed, a GOAWAY that
   excludes it, REFUSED_STREAM), and reports a failure after the response
   header without the request (Go reports it from the body's Read).
8. **HPACK and frames are C, written for this** (no library added):
   static and dynamic tables, Huffman coding from RFC 7541 Appendix B,
   an encoder that indexes as `hpack.Encoder` does. The frame code works
   on byte buffers, so `http_fuzz.c` runs it without a socket.

## Consequences

- TestH2Diff passes on the Go, LLVM, and arm64 backends: the program as
  a client of a Go HTTP/2 server and of itself (uploads and downloads
  over the windows, resets before and after the header, the 431 and 400
  answers, statuses, HEAD, faults, streams, 20 requests at once),
  net/http's client against the program's server (200 and 400 streams
  at once on one connection), and 70 raw frame cases written in the test
  (h2spec style) give the same output. TestH2Sanitizers runs the native
  build under ThreadSanitizer and AddressSanitizer with UBSan, no report,
  no allocation live at exit.
- Throughput (`bench/results/http2-2026-10-10.md`, 32 workers as streams
  of one connection): native 87.8k req/s (LLVM) and 87.0k (arm64)
  against 58.2k on the Go backend; over HTTP/1.1 with 32 TLS connections
  the same session measured 108k native and 133k Go.
- Remaining differences, documented: the native client does not
  multiplex concurrent requests on one connection; net/http waits up to
  a second for the client after a graceful GOAWAY, the native server
  closes once its streams are done; a handler natively starts once the
  request body is complete (net/http starts it with the header, and
  std/rt then reads the body first), so a request whose body overruns
  its Content-Length never reaches a native handler (net/http runs it
  and resets the stream); the native server sends each SETTINGS
  acknowledgment (net/http may merge two).
- No server push, extended CONNECT (`:protocol`), priorities, or
  trailers on either backend (net/http's defaults: push and extended
  CONNECT off; std/rt reads no trailers).

## Alternatives

- **nghttp2 or another library**: a new dependency (the task allowed
  none without a question) and its own I/O model next to the runtime's
  fibers; HPACK and framing are about 3000 lines of C here.
- **h2c**: neither net/http by default nor the spec offers it; adding
  it later is an addition (a `Server` option).
- **A reader and a writer fiber per connection**: needs one Mbed TLS
  context used from two threads at once, which Mbed TLS does not allow;
  one fiber waits on the socket and on the stream tasks in one park.
- **A multiplexing native client**: one connection for requests at once
  as Go's; more code (a reader per connection, stream routing) for no
  observable difference in sequential use, left for later.
- **Keeping HTTP/1.1 only (0106)**: the interim the owner wanted ended.
