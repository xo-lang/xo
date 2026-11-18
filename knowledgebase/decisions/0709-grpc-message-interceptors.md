# 0709: std/grpc client message interceptors (the rest of grpc-go's StreamClientInterceptor)

Status: Accepted (xo-4c, 2026-11-17; task #236)

## Context

Decision 0703 gave std/grpc interceptors. A client stream interceptor
(`Client.with_stream_interceptor`) sees and changes a call's options
before it opens, but grpc-go's StreamClientInterceptor also returns a
wrapped ClientStream whose SendMsg and RecvMsg see each message (to
count, log, sign or transform messages, or end the call). 0703 mapped
that part partial "unless ClientCall can be wrapped". ClientCall is a
concrete struct, and a wrapper value would need an interface std/grpc
does not have.

## Decision

- `fn (c: Client) with_message_interceptor[fx](on_send: fn(Str, Bytes) -> Bytes ! Status uses fx, on_recv: fn(Str, Bytes) -> Bytes ! Status uses fx) -> Client uses fx`:
  on_send runs on each message a streaming `call` sends (before the
  send limit and compression), on_recv on each message it receives;
  each gets the method and the message and returns the message to go
  on, as it came or changed, or a Status.
- An on_send error is what `send` returns, and nothing is sent; an
  on_recv error is what `recv` returns and the call ends with it (later
  `recv`s return it), as an error from a grpc-go wrapper's RecvMsg.
- Several run in the order added, the first outermost: on_send first
  to last, on_recv last to first (grpc-go's chain order).
- Effects are counted where it is registered, as for 0703's
  interceptors (pure1); the stored functions carry none.
- Unary calls are not affected (grpc-go runs only unary interceptors on
  them; `with_unary_interceptor` sees the request and the response).

## Consequences

- grpc.WithStreamInterceptor, WithChainStreamInterceptor,
  StreamClientInterceptor and Streamer map to with_stream_interceptor
  plus with_message_interceptor.
- The hooks belong to the Client, not to one call: state for one
  stream (a per-call count) lives in the caller's `f`, or the hook keys
  it by the method it is given. grpc-go's wrapper is made per call.
- The server already lets an interceptor (0703) see a stream's call,
  Recv and Send, but not wrap each message; a server message hook waits
  for an audit row that asks for it.

## Alternatives

- A ClientStream interface (trait) that interceptors wrap: Xo's call is
  a struct used by every streaming caller; making it a trait object
  changes every call site's type for one feature.
- Hooks inside CallOpts, returned by with_stream_interceptor: per-call
  closures, but closures created inside an interceptor cannot carry
  its effects in an effect-free field.
