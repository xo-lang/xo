# std/grpchealth changes

## 2026-11-17: fix: a field number over 536870911 fails to decode

- Kind: fix
- Commit: this push
- Found by: task #292: xo-4c's grpcreflect fuzz (TestGrpcReflectFuzz) found a field numbered 1,168,186,489 that
  protowire.walk reads, as protowire.ConsumeField does, and protobuf-go's proto.Unmarshal fails
- Go reference: protobuf-go internal/impl/decode.go (num > protowire.MaxValidNumber is errDecode)
- Test: std/grpchealth_test.xo (fails before: the bytes decoded)
- Shipped in: not yet released

the Check and List requests the server reads, and the responses the client reads fail with BadTag for a field numbered above 536870911 (protowire.MaxValidNumber), as protobuf-go's
proto.Unmarshal; before, the field was read (or kept as unknown). The messages are read with the new
std/protowire `fields`, walk with that check.

## 2026-11-17: feature: the gRPC health checking service (grpc.health.v1)

- Kind: feature
- Commit: this commit
- Found by: task #89 (std features line, grpc gaps in scripts/stdaudit/map.txt), decision 0546 item 5
- Go reference: google.golang.org/grpc v1.79.2 health/server.go and health/grpc_health_v1 (Apache 2.0)
- Test: std/grpchealth_test.xo (statuses, check, set, shutdown, resume, the 100 service list bound, Check, List and
  Watch over HTTP/2); internal/stdtest TestGrpcHealthInterop (Xo and grpc-go clients against Xo and grpc-go
  servers, all four pairings print the same lines)
- Shipped in: not yet released

New module: `new`, `set_serving_status`, `shutdown`, `resume`, `check`, `list` and `register` (Check, List and
Watch on a grpc.Server), and the client calls `check`, `list` and `watch`, with grpc-go's statuses, codes and texts.
