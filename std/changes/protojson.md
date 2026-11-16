# std/protojson changes

## 2026-11-16: feature: EmitUnpopulated, AllowPartial and RecursionLimit options

- Kind: feature
- Commit: this push
- Found by: task #102 (std/protobuf features): stdaudit gaps protojson.MarshalOptions.EmitUnpopulated, AllowPartial, UnmarshalOptions.AllowPartial, RecursionLimit
- Go reference: google.golang.org/protobuf v1.36.12 encoding/protojson encode.go (unpopulatedFieldRanger), decode.go (unmarshalMessage, unmarshalMap), well_known_types.go
- Test: corpus protojson: an EmitUnpopulated row per generated message in MARSHAL (375 rows, editions explicit fields included), MARSHAL2's proto2 required rows with AllowPartial, and UNMARSHAL_OPTS (33 rows: the required inputs with AllowPartial; nested messages, Struct values and Any under limits 1 to 4), on go, arm64 and LLVM
- Timing: none (option checks on paths that existed)

`Opts{emit_unpopulated: true}` writes unset fields as Go's EmitUnpopulated
does (`null` for a field with presence outside a oneof),
`Opts{allow_partial: true}` lets a message miss proto2 required fields when
marshaling and unmarshaling, and `Opts{recursion_limit: n}` limits message
nesting when unmarshaling (0 keeps protowire's default, 10000; each Value in
a Struct or ListValue counts, as in Go).

## 2026-11-16: feature: multiline output (MarshalOptions Multiline and Indent)

- Kind: feature
- Commit: this push
- Found by: task #102 (std/protobuf features): stdaudit gaps protojson.MarshalOptions.Multiline, Indent, Format
- Go reference: google.golang.org/protobuf v1.36.12 encoding/protojson encode.go, internal/encoding/json encode.go (prepareNext)
- Test: corpus protojson MULTILINE, 120 rows of protobuf-go's Multiline output (default, tab and four space indents) with its random second space after a name removed, on go, arm64 and LLVM; an indent other than spaces and tabs is an error
- Timing: none (a pass over the compact output)

`Opts{multiline: true}` and `Opts{indent: ...}` write one member per line
as protobuf-go's Multiline and Indent do.

## 2026-11-16: perf: marshal writes one builder and keeps field descriptors borrowed

- Kind: perf
- Commit: this push
- Found by: task #87 (std/evidence/protojson/perf.md, marshal 1.642 of Go on host 1), XO_PROF=cpu on bench/perf/protojson marshal and `xo dev dump ssa --stage=rc`: each FieldDesc passed down the marshal path was an owned parameter (17% in its retains), each nested message's text was built and joined into its parent's, and jstr copied and revalidated every string
- Go reference: google.golang.org/protobuf v1.36.12 encoding/protojson encode.go (one encoder buffer), internal/encoding/json/encode.go (appendString)
- Test: std/protojson_test.xo (jstr and jstr_to escapes), internal/stdtest/testdata/corpus/protojson (Go's output for every field kind, maps, well known types and the options); bench/perf/protojson on the Mac: marshal instructions 1.215G to 0.860G (0.708), unmarshal 0.96, checksums equal
- Shipped in: not yet released

`marshal` writes nested messages, member names and strings into one builder (unescaped runs of a string as they are), no longer copies the field descriptor per call (no helper on the path returns a field of it), and skips the oneof scan for a message without a real oneof. Output unchanged.

## 2026-11-10: refactor: integer bounds as `Int.max` and `Int.min` (decision 0587 point 4)

- Kind: refactor
- Commit: this commit
- Found by: decision 0587 point 4 (std's literal bounds become the type constants)
- Go reference: none (no behavior change; Go's math.MaxInt64 and MinInt64)
- Test: the std module tests and goldens of the gate; the self build

`9223372036854775807` and `-9223372036854775807 - 1` in std/protojson's code are
`Int.max` and `Int.min`; masks, Float literals, comments and texts keep their
digits. No behavior change.

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
