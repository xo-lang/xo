# std/grpchealth changes

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
