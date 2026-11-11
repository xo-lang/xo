# std/protowkt changes

## 2026-11-11: feature: the well known types derive Proto; nanos and the 32 bit wrappers are Int32 and UInt32

- Kind: feature
- Commit: this push
- Found by: decision 0549 (accepted by the owner 2026-11-11): derive expands in std modules
- Go reference: google.golang.org/protobuf v1.36.12 (timestamppb, durationpb, emptypb, fieldmaskpb, anypb, wrapperspb)
- Test: testdata/corpus/protowkt on go, arm64, llvm, with a new WIDE table (varints past 32 bits in nanos and the 32 bit wrappers, Empty with unknown fields, as protobuf-go reads and writes them back)
- Timing: not measured (not a hot path)
- Shipped in: not yet released

Timestamp, Duration, Empty, FieldMask, Any and the nine wrappers derive
Proto: their codecs are generated, `protobuf.marshal` and
`protobuf.unmarshal` take them, and `encode_NAME` and `decode_NAME` call
those. The fields protobuf-go types int32 or uint32 take Xo's Int32 and
UInt32 (`Timestamp.nanos`, `Duration.nanos`, `Int32Value.value`,
`UInt32Value.value`; were Int), the generated reader keeping a varint's
low 32 bits as Go's does. Fix: Empty kept no unknown fields, though it
declared the field; it now reads and writes them back, as protobuf-go.
Struct, Value and ListValue keep their codecs written by hand.

## 2026-11-07: port: std/protowkt, the well known types

- Kind: port
- Commit: this push
- Found by: ADR 0546 (protobuf and gRPC to full coverage), items 1 and 2
- Go reference: google.golang.org/protobuf v1.36.12 (descriptorpb, protodesc, the known types)
- Test: see below
- Timing: not measured (a codec for reflection and ProtoJSON, not a hot path yet)
- Shipped in: not yet released

Timestamp, Duration, Empty, FieldMask, Any, the wrappers, Struct, Value, ListValue and their codecs (spec/std.md 73). Test: internal/stdtest/testdata/corpus/protowkt (protobuf-go v1.36.12 marshaled deterministically, fixed and seeded random values; decoded values and byte exact re-encode) on arm64, LLVM and the Go backend.
