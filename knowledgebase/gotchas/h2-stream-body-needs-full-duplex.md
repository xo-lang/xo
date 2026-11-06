# An h2 stream handler reads the request body only with `with_full_duplex()`

Status: Observed (2026-11-06)

## Symptom

A server built on `http.stream(200, fn(w) {...})` (std/grpc's `Server.unary` and `Server.stream`) that reads the request body inside the
stream closure got no bytes, or none past the first frame, on HTTP/2: a gRPC unary call answered with its trailers missing (the client saw
the stream end with no `grpc-status`), and a client streaming call hung until its deadline. The same code worked on HTTP/1.1 and in a
handler that read the whole body before it answered.

## Cause

The server reads and answers in two phases by default: the handler's request body is finished before the response stream starts, and the
stream closure runs after it. Inside the closure the request side is already closed for the handler's reads, so a protocol that
interleaves reads and writes (gRPC's streaming, a unary call whose trailers follow the body) sees the end of the body at once, or the
response is cut when the closure returns before trailers were written.

## Fix

Declare the interleaving: `http.stream(...).with_full_duplex()`, which keeps the request body readable while the response is written.
std/native/grpc.xo `serve_call` does it for every call; set trailers with `set_trailer` before the closure returns, and end a call by
returning from the closure (a cancelled call unwinds: observe it with `defer`, not by polling `is_canceled()`).

Related: the client's `pipe` and `open` with `with_protocols(#{UnencryptedHttp2})` read trailers only after the body ended.
