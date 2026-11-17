# 0699: Response headers from a streamed body's function, and a full duplex head sent late

Status: Accepted (xo-lang-3, 2026-11-17: Go's net/http behavior, no new semantics; task #89)

## Context

A streamed response's head is fixed when `http.stream` returns: its function cannot add a header, and a full
duplex response (decision 0433, `with_full_duplex`) sends its head before the function runs. Go's handlers set
`w.Header()` until the first Write or Flush, and EnableFullDuplex does not send the head early. grpc-go's
SetHeader, SendHeader and SetSendCompressor need this (std/grpc, decision 0546; task #89).

## Decision

1. `BodyWriter.set_header(name, value) -> Bool` sets a response header (the name made canonical, as
   `with_header`) while the head has not gone out, and answers true; after it, false, and nothing changes. The
   head goes out at the first `write` (even one still buffered) or `flush`, or when the function returns, as
   net/http's first Write. A value with CR or LF is refused (false). It works the same over HTTP/1.1, HTTP/2,
   `Router.call` (a collected response) and FakeNet, on every backend.
2. `with_full_duplex` keeps sending the head at once (a client of a bidirectional exchange may wait for it
   before it sends), so `set_header` answers false in its function. `Response.with_late_head()` is full duplex
   with the head left to the first write or flush, as Go's EnableFullDuplex without a Flush.
3. std/grpc serves calls with `with_late_head`, and `Send.set_header(name, value)` and
   `Send.send_header(md)` are grpc-go's SetHeader and SendHeader: metadata until the first message or
   send_header, then `Internal` "transport: SendHeader called multiple times" (ErrIllegalHeaderWrite).
4. Native: the header goes into the connection's header table (llrt hserve `sink_set_header`) while the sink
   has not started and holds no byte; the wasm and in memory paths keep the head in a `HeadPend` behind a
   Mutex. Go backend (std/rt): the head is written by a start-once function at the first write or flush or
   the end of the function, and `set_header` writes `w.Header()` until then.

## Consequences

- A handler can decide headers from work it does while streaming (a status known after a lookup, a
  compressor chosen per call) without buffering the body.
- `with_full_duplex`'s behavior is unchanged; code that needs headers from the function uses
  `with_late_head`, whose client sees no head until the first byte or flush.
- Test: internal/llvm/testdata/golden/http_set_header (h1, h2, Router.call; Go, native and LLVM agree), and
  the grpc interop's md line against grpc-go.

## Alternatives

- Make `with_full_duplex` send the head late: a bidirectional client waiting for the head before it sends
  would hang (std/grpc's own request body was discarded when tried, 2026-11-17).
- A header callback run before the function: cannot use what the function learns while streaming.
