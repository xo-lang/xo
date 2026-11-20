# 0728: a multiplexing native HTTP/2 client

Status: Proposed (xo-4c, 2026-11-20, task #494; the owner decides, xw
ask filed by the lead). Would supersede the "A multiplexing native
client ... left for later" alternative and the first "Remaining
differences" item of decision 0121. No code until the owner answers.

## Context

Decision 0121 gave both backends HTTP/2 and left one difference with
Go's client documented: the native client does not multiplex. Each
request takes a connection out of the pool for itself
(std/native/llrt/hclient.xo `exchange_start`, one exchange record `x`
per connection) and gives it back in `h2_end` once its stream has ended
(the response body read to END_STREAM, or the stream reset). The
pump (std/native/llrt/http1.xo `h2_pump`) reads the connection for that
one stream: a frame for any other stream id is counted for flow control
and the HPACK table and then dropped ("an old stream's").

0121 judged this as "no observable difference in sequential use". Task
#474 found where it is observable:

- A response whose body is not read yet keeps its connection out of the
  pool, so the next request to the same origin dials a new connection
  (http_idle_pings' `held` case saw 2 connections on linux hosts 3 and
  4 until the golden read the body first, 2026-11-20). Go opens a
  second stream on the first connection.
- Requests made at once from several tasks to one origin open one
  connection each; Go's Transport opens one connection and runs them as
  concurrent streams up to the server's SETTINGS_MAX_CONCURRENT_STREAMS
  (then waits, or with StrictMaxConcurrentStreams false dials another).
- So a client fanning out to one HTTP/2 server pays one TCP and TLS
  handshake per request in flight, holds one socket each, and a server
  limiting connections per client (common for gRPC and API gateways)
  sees many where Go shows one. std/grpc's client inherits this.

## Decision (proposed)

The native HTTP/2 client multiplexes requests on one connection per
origin, as golang.org/x/net/http2's ClientConn:

1. **A reader per connection.** A connection gets one read loop (a task
   in the client's group, as the idle pinger of `with_idle_pings` is),
   which reads every frame and routes it by stream id to that stream's
   record: HEADERS and CONTINUATION (HPACK decoded on the reader, the
   table being per connection), DATA into the stream's body buffer
   with flow control per stream and per connection, RST_STREAM, and
   connection frames (SETTINGS, PING acks, WINDOW_UPDATE, GOAWAY)
   handled on the connection. Writes go through one writer lock per
   connection, so HEADERS plus CONTINUATION stay contiguous.
2. **Streams, not connections, are taken.** `exchange_start` reserves a
   stream on a pooled connection that has room (open streams below the
   peer's MAX_CONCURRENT_STREAMS, not GOAWAY, not broken) and only
   dials when none has; a connection stays in the pool while its
   streams run (Go's `idleStateLocked`/`ReserveNewRequest`). A request
   waits for a stream when the limit is reached and
   `H2Config.strict_max_concurrent_streams` is true, else dials
   (Go's default, StrictMaxConcurrentStreams false).
3. **Bodies read from the stream buffer.** `h2_take_body` and the body
   reader read the stream's buffer, which the reader fills and whose
   WINDOW_UPDATEs follow the reads (Go's `transportResponseBody`), so
   an unread body holds flow-control window, not the connection.
4. **Retry rules as Go's.** A request on a connection that got GOAWAY
   with a last stream id below its own, or REFUSED_STREAM, is retried
   on another connection when its body can be replayed
   (`canRetryError`, `shouldRetryRequest`).
5. **Lifetime.** The idle pinger (`Client.with_idle_pings`, task #474)
   pings a connection with no open streams; a connection with streams
   relies on ReadIdleTimeout as Go does. A connection closes when idle
   past the pool's idle timeout with no streams, or on GOAWAY once its
   last stream ends (TestTransportClosesConnAfterGoAwayLastStream).
6. **wasm.** The WASI 0.2 component client goes through wasi:http,
   which the host multiplexes; the native reader is behind a
   compile-time target test of its own (gotchas/
   wasm-runtime-zero-guard-keeps-callee-reachable.md), and wasm keeps
   today's code.

### Tests (ported from golang.org/x/net/http2 v0.62.0, transport_test.go and transport_api_test.go)

- Concurrency limits: TestAPITransportStrictMaxConcurrentStreamsEnabled,
  ...Disabled; a golden of N tasks to one h2c server reporting one
  connection and N concurrent streams (server side counts).
- The #474 case: a request while an earlier response body is unread
  uses the same connection (http_idle_pings' `held` without its body
  read, connections 1).
- Flow control per stream and connection: TestTransportFlowControl,
  TestTransportReturnsUnusedFlowControlSingleWrite and
  ...MultipleWrites, TestTransportAdjustsFlowControl,
  TestTransportReturnsDataPaddingFlowControl,
  TestTransportSettingsFlowControlUpdateBeyondLimit and
  ...WithinLimit, TestTransportWindowUpdateBeyondLimit.
- GOAWAY and retries: TestTransportRetryAfterGOAWAYNoRetry, ...Retry,
  ...SecondRequest, TestTransportRetryAfterRefusedStream,
  TestTransportClosesConnAfterGoAwayNoStreams and ...LastStream,
  TestTransportNoRetryOnStreamProtocolError.
- Streams ending early: TestTransportServerResetStreamAtHeaders,
  TestTransportResponseAndResetWithoutConsumingBodyRace,
  TestTransportBodyEagerEndStream, TestTransportBodyDoubleEndStream,
  TestClientConnCloseAtBody, TestTransportStreamEndsWhileBodyIsBeingWritten.
- Idle and pings: TestIdleConnTimeout, TestTransportSendPingWithReset,
  TestTransportNoPingAfterResetWithFrames.
- Evidence after the port (owner rule, 2026-11-15): stress of many
  tasks on one connection under XO_THREADS=2 and 8, the race build, an
  interop run against Go's h2 server and against an h2c Go client's
  server, and `xo bench --ab` of a fan-out client (one origin, 64
  requests in flight) against today's client and Go's Transport.

## Consequences

- One connection per origin under concurrency, as Go: fewer handshakes
  and sockets for fan-out clients, std/grpc included; a server's
  per-client connection limit sees what it sees from Go.
- More code in std/native/llrt (a reader task, stream routing, per
  stream buffers and windows): roughly the size of today's h2 pump
  again. hclient.xo (2,131 lines) and http1.xo (5,306) are the files
  touched; the HTTP/1.1 path is unchanged.
- A connection failure now fails every stream on it, as Go's; a slow
  reader of one body no longer blocks other requests, but one stream's
  unread body holds its window (65,535 bytes by default) on the shared
  connection, as Go's.
- Sequential use behaves as today; the existing h2 goldens stay valid.
- spec/std.md (Client, H2Config) gains strict_max_concurrent_streams;
  0121's documented difference is removed there and in the std/http
  docs.

## Alternatives

- **Keep 0121's single-stream client** (today): no new code; fan-out
  clients keep one connection per request in flight, and an unread
  body keeps its connection out of the pool.
- **Pool reuse only after headers** (smaller): give the connection
  back once the response headers arrive and let the body be read by
  the pump of whichever request holds it next. Rejected: frames of two
  streams on one connection still need routing by stream id, which is
  the reader of the decision without its clean ownership.
- **Several connections per origin, no multiplexing** (as HTTP/1.1):
  what happens today by accident; it does not match Go and wastes
  connections under HTTP/2's design.
