# std/protodesc changes

## 2026-11-07: port: std/protodesc, descriptor.proto as Xo values

- Kind: port
- Commit: this push
- Found by: ADR 0546 (protobuf and gRPC to full coverage), items 1 and 2
- Go reference: google.golang.org/protobuf v1.36.12 (descriptorpb, protodesc, the known types)
- Test: see below
- Timing: not measured (a codec for reflection and ProtoJSON, not a hot path yet)
- Shipped in: not yet released

`protodesc.decode` and `encode` (spec/std.md 72). Hand written codec over std/protobuf (derive Proto does not expand in std, decision 0549). Test: internal/stdtest/testdata/corpus/protodesc (descriptor.proto, the well known types, grpc-go health, reflection and test descriptors: decode, counts, byte exact re-encode) on arm64, LLVM and the Go backend; `xo test --std-module=native/protodesc`.
