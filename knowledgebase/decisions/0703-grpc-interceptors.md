# 0703: std/grpc interceptors, with effects counted where they are registered

Status: Accepted (owner, 2026-11-17, xw decision 66; task #236)

## Context

scripts/stdaudit/map.txt has 18 interceptor rows (grpc.UnaryInterceptor,
StreamInterceptor, ChainUnaryInterceptor, ChainStreamInterceptor, the
client With*Interceptor options, and the UnaryHandler, StreamHandler,
UnaryInvoker, Streamer, UnaryServerInfo and StreamServerInfo types) and
grpc.behavior.interceptors as gaps ("wrap the handler function by
hand"). grpc-go chains server interceptors around every method and
client interceptors around every call, in registration order, the first
outermost.

An interceptor is stored and calls `next`, the handler, whose effects
differ per method. std/http answered the same question for middleware
(spec std.md 4.x, `Router.wrap`): a middleware's effects are counted
where it is registered, and `Next` carries none in its type.

## Decision

std/grpc follows std/http's rule.

Server:
- `type StreamNext = fn(Call, Recv, Send) -> () ! Status` and
  `type UnaryNext = fn(Call, Bytes) -> Bytes ! Status`, with no effects
  in the types, as `http.Next`.
- `fn (var s: Server) intercept[fx](f: fn(Call, Recv, Send, StreamNext) -> () ! Status uses fx) uses fx`
  (grpc-go's StreamInterceptor) runs around every method, unary ones
  included (a unary method is a stream with one message each way).
- `fn (var s: Server) intercept_unary[fx](f: fn(Call, Bytes, UnaryNext) -> Bytes ! Status uses fx) uses fx`
  (UnaryInterceptor) runs around unary methods only, inside the stream
  interceptors.
- Several of each chain in registration order, the first registered
  outermost (ChainUnaryInterceptor, ChainStreamInterceptor). They apply
  to every method, registered before or after them, and to `unknown`'s
  handler: the Server keeps each method's handler and `router()` wraps
  them when it builds the routes.
- The method's kind, grpc-go's StreamServerInfo (IsClientStream,
  IsServerStream), is in `Call` (`client_stream`, `server_stream`, as
  `services` lists it).

Client:
- `type Invoker = fn(Str, Bytes, CallOpts) -> Bytes ! Status` (method,
  request, the call's metadata and timeout) and
  `fn (c: Client) with_unary_interceptor[fx](f: fn(Str, Bytes, CallOpts, Invoker) -> Bytes ! Status uses fx) -> Client uses fx`
  around `unary`; a stream interceptor around `call`
  (StreamClientInterceptor) gets the method and options and may change
  them before the call opens; one that wraps the ClientCall's send and
  recv is mapped partial unless ClientCall can be wrapped.

## Consequences

- Effects are checked where the interceptor is registered, as for http
  middleware: the function that builds the server declares them.
- grpc-go runs either the unary or the stream interceptor for a method,
  never both; std/grpc nests unary ones inside stream ones, so one
  authentication interceptor covers both kinds. Noted in spec/std.md as
  a difference.
- Tests port what fits of grpc-go's interceptor tests (chain order, a
  refusal with a status, metadata changed on the way in, Recv wrapped
  to count messages) and add an interop line with a grpc-go client.

## Alternatives

- Wrap the handler by hand (today): what the map rows call a gap.
- A Server generic over effects (`Server[fx]`): every Server type would
  carry effects; std/http chose not to for Router.
- Interceptors as http middleware (`Router.wrap`): they see Requests,
  not Call, Recv and Send, so they cannot read or replace messages.
