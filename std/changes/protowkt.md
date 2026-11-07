# std/protowkt changes

## 2026-11-07: port: std/protowkt, the well known types

- Kind: port
- Commit: this push
- Found by: ADR 0546 (protobuf and gRPC to full coverage), items 1 and 2
- Go reference: google.golang.org/protobuf v1.36.12 (descriptorpb, protodesc, the known types)
- Test: see below
- Timing: not measured (a codec for reflection and ProtoJSON, not a hot path yet)
- Shipped in: not yet released

Timestamp, Duration, Empty, FieldMask, Any, the wrappers, Struct, Value, ListValue and their codecs (spec/std.md 73). Test: internal/stdtest/testdata/corpus/protowkt (protobuf-go v1.36.12 marshaled deterministically, fixed and seeded random values; decoded values and byte exact re-encode) on arm64, LLVM and the Go backend.
