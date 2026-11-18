# std/protojson changes

## 2026-11-18: fix: a Duration may leave out its integer part or its fraction's digits

- Kind: fix
- Commit: this push
- Found by: TestProtojsonFuzz seed 179 (Mac, arm64 and llvm): Go read "+.000001s", Xo refused it
- Go reference: google.golang.org/protobuf v1.36.12 encoding/protojson parseDuration
- Test: std/protojson_test.xo "a Duration may leave out its integer part or its fraction's digits,
  as in Go" (failed before)
- Shipped in: not yet released

A Duration string such as ".5s" or "+.000001s" (no digit before the period) and "1.s" or ".s"
(no digit after it) is read as Go reads it: 0.5 s, 1 µs, 1 s and 0 s. Each was an error before.

## 2026-11-18: fix: an Any's well known type value counts against RecursionLimit as in Go

- Kind: fix
- Commit: this push
- Found by: the protojson fuzz harness (TestProtojsonFuzz, linux/arm64, "unmarshal with AllowPartial and RecursionLimit equals Go's protojson"): at limit 2 Go read a message holding an Any of a Timestamp, Xo failed it
- Go reference: google.golang.org/protobuf v1.36.12 encoding/protojson well_known_types.go unmarshalAny, unmarshalAnyValue, findTypeURL and skipJSONValue
- Test: std/protojson_test.xo "a well known type in an Any's value does not count against the recursion limit, as in Go" (fails before); results from Go's protojson.Unmarshal
- Shipped in: not yet released

The Any counts one level of RecursionLimit, and a well known type in its "value" no longer counts another. As in Go, an object or array in a member other than "@type" may nest only as deep as the levels left (an Any of Empty with "value":{} fails at limit 1).

## 2026-11-17: fix: a Timestamp's fraction may follow a comma, as in Go

- Kind: fix
- Commit: this push
- Found by: the protojson fuzz harness (TestProtojsonFuzz, darwin/arm64): Go's Unmarshal read "9999-12-31T23:59:59,120Z", Xo's failed it
- Go reference: google.golang.org/protobuf encoding/protojson well_known_types.go unmarshalTimestamp (time.Parse with RFC3339Nano, which takes ',' or '.')
- Test: std/protojson_test.xo "a Timestamp's fraction may follow a comma, as Go's time.Parse reads it" (fails before); values from Go's protojson.Unmarshal
- Shipped in: not yet released

A Timestamp's fractional seconds may follow a comma as well as a period. As in Go, more than 9 digits fail only after a period; after a comma the first 9 are kept.

## 2026-11-17: fix: a field number over 536870911 fails to decode

- Kind: fix
- Commit: this push
- Found by: task #292: xo-4c's grpcreflect fuzz (TestGrpcReflectFuzz) found a field numbered 1,168,186,489 that
  protowire.walk reads, as protowire.ConsumeField does, and protobuf-go's proto.Unmarshal fails
- Go reference: protobuf-go internal/impl/decode.go (num > protowire.MaxValidNumber is errDecode)
- Test: std/protojson_test.xo (fails before: the bytes decoded)
- Shipped in: not yet released

`marshal` and `normalize` of wire bytes fail with BadTag for a field numbered above 536870911 (protowire.MaxValidNumber), as protobuf-go's
proto.Unmarshal; before, the field was read (or kept as unknown). The messages are read with the new
std/protowire `fields`, walk with that check.

## 2026-11-17: feature: proto2 extensions

- Kind: feature
- Commit: this push
- Found by: task #228 (follow-up of std/prototext's reader, #153)
- Go reference: google.golang.org/protobuf v1.36.12 encoding/protojson (encode.go marshalMessage with order.IndexNameFieldOrder, decode.go unmarshalMessage's `[name]` keys)
- Test: testdata/corpus/protojson, generated from protobuf-go: pjt2.Ext with four file scope extensions and one declared in Leaf; marshal rows (extensions after fields, by full name, a message extension, a zero value) and unmarshal rows (unknown name, duplicate, null, a message extension missing a required field, a message that cannot take the extension)
- Timing: none (one map lookup per message with extensions)

The registry now keeps the extensions its files declare. A set extension is written as `"[full.name]"` after the
fields, sorted by full name, and read back from that key; an extension the message cannot take is an error as in Go.
## 2026-11-17: fix: an integer map key is read as Go reads it

- Kind: fix
- Commit: this push
- Found by: `stdproven fuzz protojson` (TestProtojsonFuzz seed 2: an int32 map key "-20735673E0" was
  accepted; Go refuses it)
- Go reference: google.golang.org/protobuf encoding/protojson decode.go unmarshalMapKey (strconv.ParseInt
  and ParseUint, base 10)
- Test: std/protojson_test.xo "an integer map key takes a sign and leading zeros, never an exponent,
  fraction or space" (failed before: the key read as -20735673)
- Shipped in: not yet released

An integer map key is an optional sign (+ or -, signed kinds only) and decimal digits, leading zeros
allowed; one with an exponent, a fraction or a space is an error, as in Go. It was read as a quoted
number value, which allows those, and "007" was refused.

## 2026-11-17: perf: field_to finds a field's occurrences by binary search

- Kind: perf
- Commit: this push
- Found by: task #87 (Performance Early to Proven), bench/perf/protojson
- Go reference: google.golang.org/protobuf v1.36.12 encoding/protojson encode.go (fields in number order)
- Test: corpus protojson (MARSHAL, MARSHAL2, UNMARSHAL_OPTS) on go, arm64 and LLVM; std/protojson module tests
- Timing: host 1, xo bench --ab bench/perf/protojson, 3 rounds: wall 45.44 ms to 44.60 ms (B/A 0.981, 95% interval 0.977 to 0.981), RSS unchanged

`field_to` sorts the wire's fields by number once and finds each field's
occurrences by binary search, where it scanned the whole message per field.

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
