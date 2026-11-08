# std/protojson changes

## 2026-11-08: feature: editions 2023 descriptors, and groups read and written as JSON objects

- Kind: feature
- Commit: this push
- Found by: the protobuf conformance runner's editions test messages (843 tests the testee answered `skipped`), test/protoconf
- Go reference: google.golang.org/protobuf v1.36.12 (protodesc editions, encoding/protojson, dynamicpb)
- Test: testdata/corpus/protojson: two editions 2023 files in the oracle schema (pjted.proto: per-field presence IMPLICIT and
  LEGACY_REQUIRED, repeated EXPANDED and PACKED, message_encoding DELIMITED, an implicit file with an EXPLICIT field), random
  messages of them in MARSHAL, UNMARSHAL and NORMALIZE against Go on arm64, LLVM and the Go backend; the conformance runner of
  protobuf v29.3 over the four test message files: 5348 successes, 0 failures (was 2674 with proto3 and proto2 only)
- Timing: not applicable
- Shipped in: not yet released

A file with `syntax: editions` is read as its proto2 or proto3 form: the FeatureSet of the file, message and field
(descriptor.proto fields 50, 12, 21) resolves field presence (a LEGACY_REQUIRED field becomes required, an IMPLICIT one in an
explicit message is implicit, an EXPLICIT one in an implicit message explicit), repeated number encoding (the `packed`
option), and message_encoding DELIMITED (a message field becomes a group). Not read: enum_type (closed enums) and
utf8_validation. Groups, which the JSON side refused before ("groups are not supported"), are now read and written as
JSON objects (`native/protodesc.walk` gives a group field's contents as `data`); the group's start and end tags are written
on the wire. `MsgInfo` gains `implicit_nums`.

## 2026-11-08: feature: normalize keeps a message's unknown fields; fix: a wrong wire type no longer decides a oneof

- Kind: feature
- Commit: this push
- Found by: the protobuf conformance runner's two listed failures (UnknownVarint and UnknownOrdering, protobuf to protobuf), test/protoconf
- Go reference: google.golang.org/protobuf v1.36.12 (proto.Unmarshal then proto.Marshal on a dynamicpb message)
- Test: testdata/corpus/protojson NORMALIZE (random messages of the corpus schema, each alone, with an unknown field before, after and around its known fields, with a known field sent in the wrong wire type, truncated, and empty; Go's bytes or error), on arm64, LLVM and the Go backend
- Timing: not applicable
- Shipped in: not yet released

`protojson.normalize(r, name, wire)` returns the bytes protobuf-go writes after parsing `wire`: the known fields in canonical form,
then the message's own unknown fields (a number the descriptor lacks, or a wire type it does not take) as they were on the wire.
It is built on `marshal` and `unmarshal`, so it is exact for a message the JSON mapping carries without loss (the corpus leaves
out an Any with no type URL, for one), and unknown fields inside nested messages are not kept. The conformance testee can use it
for protobuf to protobuf requests.

The same work found a `marshal` bug: a oneof member that arrived with the wrong wire type counted as the member that was set
last, so the real member was left out of the JSON; such a field is now unknown and does not displace it.

## 2026-11-08: fix: proto2 required fields are checked

- Kind: fix
- Commit: this push
- Found by: the protobuf conformance runner with TestAllTypesProto2 (test/protoconf)
- Go reference: google.golang.org/protobuf v1.36.12 (encoding/protojson, proto.CheckInitialized)
- Test: testdata/corpus/protojson (proto2 Req and Leaf: 12 unmarshal inputs and 9 marshal wires, Go's result or error)
- Timing: not applicable
- Shipped in: not yet released

`marshal` of wire bytes missing a proto2 `required` field, and `unmarshal` of JSON missing one (or giving `null`), now fail with `required field NAME not set`, at any depth, as protobuf-go does; before, both succeeded. Conformance (test/protoconf, proto3 and proto2 messages): 2670 successes, 4 listed failures (unknown fields of protobuf input re-serialized through JSON), 0 unexpected.

## 2026-11-08: fix: marshal and unmarshal without the options on native builds

- Kind: fix
- Commit: this push
- Found by: Xo Lang 3 (the conformance testee, test/protoconf)
- Go reference: none (a compiler limit, not a protojson behavior)
- Test: internal/llvm/testdata/golden/std_struct_default_arg (arm64, LLVM, Go backend)
- Timing: not applicable
- Shipped in: not yet released

`protojson.marshal(r, name, wire)` with the `o: Opts = Opts{}` argument left out failed to build on the native backends with XO1201 ("a struct literal without the field `proto_names`"): the declaration file's default literal has no type recorded by the checker, so the std struct's field defaults were not found. The SSA builder now takes the type from the parameter's declaration (compiler/ssa call.xo and expr.xo), so any std function with a struct literal default works on native as on the Go backend.

## 2026-11-07: port: std/protojson, the canonical JSON mapping

- Kind: port
- Commit: this push
- Found by: ADR 0546 (protobuf and gRPC to full coverage), item 2
- Go reference: google.golang.org/protobuf v1.36.12 (encoding/protojson)
- Test: see below
- Timing: not measured yet
- Shipped in: not yet released

Descriptor driven marshal and unmarshal of any message (spec/std.md 74), with the well known types' special forms. Test: internal/stdtest/testdata/corpus/protojson (protojson marshal of fixed and random messages, byte identical after compaction; unmarshal of curated and random inputs, equal wire bytes, rejected inputs matched) on arm64, LLVM and the Go backend. Google's protobuf conformance runner (v29.3, test/protoconf, bench/results/protobuf-conformance-2026-11-07.md): 1411 successes and 2 listed failures against protobuf-go's 1412 and 0; the run found sint64's extremes faulting in the encoder, fixed here. Skipped as gaps: text format, jspb and proto2 message types.
