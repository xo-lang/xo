# std/grpc changes

## 2026-11-06: feature: grpc.bin_encode and grpc.bin_decode for `-bin` metadata

- Kind: feature
- Commit: this commit
- Found by: decision 0535's list of what the first version leaves out
- Go reference: PROTOCOL-HTTP2.md (Custom-Metadata: a name ending in `-bin` carries base64, written unpadded, read padded or not); grpc-go internal/metadata
- Test: std/native/grpc_test.xo "binary metadata is unpadded base64 and reads padded or not" (RFC 4648 vectors, 40 length round trips)
- Shipped in: not yet released

A metadata value of a name ending in `-bin` is bytes: `bin_encode(data)` makes the header text, `bin_decode(text)` reads one from a peer.

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
