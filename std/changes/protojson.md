# std/protojson changes

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
