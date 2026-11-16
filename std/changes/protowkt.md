# std/protowkt changes

## 2026-11-16: feature: field masks against a derive Proto type's descriptor

- Kind: feature
- Commit: this push
- Go reference: google.golang.org/protobuf v1.36.12 types/known/fieldmaskpb (New, IsValid, Append; numValidPaths)
- Test: corpus protowkt fieldmask_typed_test.xo: every path of up to three segments from 13 names gets is_valid's answer on the same message parsed from .proto text (which fieldmask_test.xo checks against fieldmaskpb's tables), and new_mask's and append's paths and errors; arm64, llvm and go
- Shipped in: not yet released

`new_field_mask(md, paths)`, `FieldMask.append_paths(md, paths)` and `FieldMask.is_valid_for(md)` take `protobuf.descriptor[T]()` (decision 0683 item 2), so a derive Proto type's masks need no .proto file. ADR 0683 named them `new_field_mask[T]`, `is_valid_mask[T]` and `append[T]`: a std function cannot pass its type parameter on to `protobuf.descriptor` (XO0427: derive Proto's functions need the concrete type at the call), so they take the descriptor the caller gets with the type.

## 2026-11-16: fix: Timestamp.as_time near Int's range, and Value.to_json of NaN and the infinities

- Kind: fix
- Commit: this push
- Found by: corpus protowkt's new rows against protobuf-go (TestStdAPICovered coverage, task #89)
- Go reference: timestamppb's AsTime (time.Unix carries nanos into the seconds first); structpb's Value.AsInterface (NaN and the infinities as the strings "NaN", "Infinity", "-Infinity")
- Test: corpus protowkt TS_CHECKS (Timestamp{seconds: 9223372037, nanos: -999999999} and 25 more were None, Go gives the instant) and VALUE_JSON, STRUCT_JSON, LIST_JSON (25 rows with an infinity were numbers); both failed before and pass after, on arm64, llvm and go
- Shipped in: not yet released

`as_time` carries the nanos into the seconds before multiplying, as time.Unix does, so an instant within Int's nanoseconds is no longer None when the seconds alone are past it. `to_json` gives the strings Go's AsInterface gives for NaN and the infinities instead of a number JSON cannot hold.


## 2026-11-16: feature: field mask New, IsValid and Append against a descriptor

- Kind: feature
- Commit: this push
- Found by: task #109 (the partial Go row of std/protowkt): stdaudit gaps fieldmaskpb.New, FieldMask.IsValid, FieldMask.Append
- Go reference: google.golang.org/protobuf v1.36.12 types/known/fieldmaskpb (New, IsValid, Append, numValidPaths)
- Test: std/protowkt_test.xo, field_mask_test.go's TestAppend and TestIsValid over protobuf-go's internal/testprotos/test/test.proto (a FileDescriptorSet from protoc), on arm64 and LLVM
- Timing: none (one pass over the paths given, each segment a scan of one message's fields)

`new_mask(files, name, paths)`, `FieldMask.append(files, name, paths)` and `FieldMask.is_valid(files, name)` check a
mask's paths against message `name` of std/protodesc files, as fieldmaskpb does against a generated message's
descriptor: each segment a field of the message the one before it holds, a repeated or map field only last, a group named
by its message's name. `append` keeps the paths before the first invalid one; the error is protobuf-go's text.

## 2026-11-16: feature: protobuf-go's types/known helpers

- Kind: feature
- Commit: this push
- Found by: task #102 (std/protobuf features): stdaudit gaps timestamppb.*, durationpb.*, anypb.*, structpb.*, wrapperspb.*, emptypb.*, fieldmaskpb.*
- Go reference: google.golang.org/protobuf v1.36.12 types/known (timestamppb, durationpb, anypb, fieldmaskpb, structpb)
- Test: std/protowkt_test.xo, protobuf-go's timestamp_test.go, duration_test.go, field_mask_test.go (TestCombine, TestNormalize) and any_test.go (MessageIs, MessageName) cases, on arm64 and LLVM; the Go backend builds the module (corpus protowkt)
- Timing: none (no loops over input beyond the paths and JSON values given)

`timestamp_of`, `Timestamp.as_time`, `is_valid`, `check_valid`;
`duration_from_nanos`, `Duration.as_nanos`, `is_valid`, `check_valid`;
`pack_any`, `Any.unpack`, `message_is`, `message_name`; `FieldMask.normalize`,
`union_masks`, `intersect_masks`; `value_from_json`, `struct_from_json`,
`list_from_json` and `to_json` on Value, Struct and ListValue. Errors are
`WktErr` with protobuf-go's texts.

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
