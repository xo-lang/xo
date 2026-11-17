# 0706: xo proto gen writes typed gRPC stubs for service blocks

Status: Accepted (xo-lang-3, 2026-11-17; task #236's stub rows, agreed with xo-4c; protoc-gen-go-grpc's output, no new semantics)

## Context

`xo proto gen` (decision 0530) writes messages and enums only; a `service` block is parsed and kept in the
descriptor but produces no Xo. grpc-go's protoc-gen-go-grpc writes a typed client (one method per rpc) and a
register function over a ServiceDesc. The map.txt rows grpc.ServiceDesc, MethodDesc, StreamDesc, MethodHandler,
ServiceRegistrar and ClientConnInterface are gaps for that reason.

## Decision

For each service `S` of package `p`, `gen_set` (std/protoparse, `xo proto gen`) writes, after the file's messages
and enums, with `use std/grpc` and `use std/protobuf` at the top of that file only:

1. `pub const S_M: Str = "/p.S/M"` per rpc (names in snake case, the constant upper case).
2. `pub struct SClient { pub c: grpc.Client }` and `new_s_client(c)`, with one method per rpc: unary
   `m(req, metadata = {}, timeout = None) -> Resp ! grpc.Status`; server streaming `m(req, f: fn(Resp) -> Bool uses
   fx)` (false stops reading, the call then ends OK, as std/grpchealth's watch); client streaming
   `m(reqs: List[Req]) -> Resp`; bidirectional `m(f: fn(SMStream) -> R ! grpc.Status uses fx) -> R`, the typed
   stream having `send`, `close_send` and `recv`.
3. `handle_s_m(var srv: grpc.Server, f)` per rpc, with f typed by the rpc's kind: `fn(grpc.Call, Req) -> Resp`,
   `fn(grpc.Call, Req, SMSend)`, `fn(grpc.Call, SMRecv) -> Resp` or `fn(grpc.Call, SMRecv, SMSend)`, each `! grpc.Status
   uses fx`; `SMRecv.next() -> Req?` and `SMSend.send(Resp)` wrap std/grpc's Recv and Send.
4. Only std/grpc's public calls are used (Client.unary and call, Server.unary and stream), so interceptors,
   per-RPC credentials, the service config's retries and limits apply to the stubs as to hand written calls. A
   message that does not decode is `Internal` with grpc-go's texts.

## Consequences

- The six map.txt rows (ServiceDesc, MethodDesc, StreamDesc, MethodHandler, ServiceRegistrar, ClientConnInterface)
  map to the generated code and the std/grpc calls it makes.
- A service's handlers register one rpc at a time; a handler of every rpc is not checked at compile time (grpc-go's
  Unimplemented server embeds a default; here an rpc never registered answers Unimplemented, as std/grpc does).
- TestProtoGenServiceRuns (cmd/xo) generates a service with the four kinds of rpc and serves and calls each.

## Alternatives

- A struct of handler functions and one `register_s`: Xo function fields cannot carry their effects (std/grpc's
  interceptors store theirs through `pure1`, decision 0703), so each handler's effects would be erased or fixed;
  one generic `handle_` per rpc keeps them.
- Generic typed helpers in std/grpc (`typed_unary[Req, Resp]`) instead of generated wrappers: the same calls with
  more API in std/grpc; generated code keeps std/grpc's surface as it is.
