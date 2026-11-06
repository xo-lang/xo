# std/grpc changes

## 2026-11-06: feature: std/grpc, gRPC over std/http's HTTP/2 (decision 0535)

- Kind: feature
- Commit: this commit
- Found by: owner decision 2026-11-06 (gRPC after protobuf, before OTLP export)
- Go reference: google.golang.org/grpc v1.79.2 (the interop oracle, internal/stdtest/grpcref), PROTOCOL-HTTP2.md
- Test: std/native/grpc_test.xo (frame, timeout and message codecs, random byte reads), testdata/corpus/grpc (grpc-timeout headers and status messages as grpc-go sends and decodes them), golden grpc_calls (unary, status, unknown method, a server deadline, server, client and bidirectional streams), TestGrpcInterop (an Xo client against a grpc-go server and a grpc-go client against an Xo server, the same six calls)

New module: the 17 status codes, `Status`, the message frame, `grpc-timeout` and `grpc-message` coding, a `Server` that
registers unary and streaming handlers and mounts on `http.server`, and a `Client` with `unary` and `call` for the
three streaming shapes. Deadlines cancel a server handler. Unfinished against decision 0535: compression, binary metadata,
extra response metadata, `xo proto gen` stubs, fuzzing of the trailer parser beyond the random byte tests, a timing row.
