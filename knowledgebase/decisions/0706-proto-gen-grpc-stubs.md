# 0706: xo proto gen writes typed gRPC stubs for service blocks

Status: Proposed (xo-lang-3, 2026-11-17; task #236's stub rows, agreed with xo-4c)

## Context

`xo proto gen` (decision 0530) writes messages and enums only; a `service` block is parsed and kept in the
descriptor but produces no Xo. grpc-go's protoc-gen-go-grpc writes a typed client (one method per rpc) and a
register function over a ServiceDesc. The map.txt rows grpc.ServiceDesc, MethodDesc, StreamDesc, MethodHandler,
ServiceRegistrar and ClientConnInterface are gaps for that reason.

## Decision

To be filled in: for each service S of package p, the generated source holds (only public std/grpc API is
used, so retries, interceptors and credentials apply as to hand written calls):

1. `pub const S_SERVICE: Str = "p.S"` and one `/p.S/Method` constant per rpc.
2. A typed client `SClient{c: grpc.Client}` with one method per rpc: unary `m(req) -> Resp ! grpc.Status`,
   server streaming `m(req, f: fn(Resp) -> Bool)`, client streaming `m(reqs: List[Req]) -> Resp`, bidirectional
   `m(f: fn(SMStream) -> R ! grpc.Status)` over a generated typed stream (send, recv, close_send).
3. One register function per rpc taking a typed handler (`handle_s_m(var srv, f)`), since a struct of handler
   functions cannot carry their effects; a `register_s` is not generated.

## Consequences

To be filled in.

## Alternatives

To be filled in.
