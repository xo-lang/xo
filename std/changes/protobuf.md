# std/protobuf changes

## 2026-11-06: feature: std/protobuf, the wire layer and derive Proto (decision 0530 Accepted)

- Kind: feature
- Commit: this commit
- Found by: owner queue (Xo Lead): protobuf, then gRPC, then OTLP
- Go reference: google.golang.org/protobuf (protowire, proto, dynamicpb) v1.36.12, the differential oracle
- Test: testdata/corpus/protobuf (wire layer cases, typed marshal and unmarshal cases for scalars, Option, List, nested messages, maps, enums, oneofs, scalar widths and unknown fields against dynamicpb), bench/perf/protobuf (timing against protoc-gen-go messages), std/native/protobuf_test.xo

New module. The wire layer (`append_varint`, `append_tag`, `append_fixed32`, `append_fixed64`, `append_bytes`, the
`read_*` functions, `skip_field`, zigzag) follows protowire, including its error cases and the group recursion limit.
`derive Proto` generates `protobuf.marshal` and `protobuf.unmarshal` for structs with a `proto "N"` field tag: proto3
implicit presence, `Option` for explicit presence, packed `List`, nested messages (merged like Go), `Map` (keys sorted
like a deterministic marshal), Int valued enums, oneofs, the scalar widths Int32, UInt32, UInt64 and Float32 (with `zigzag` and `fixed`), and unknown
fields kept in a `proto "unknown"` Bytes field. Not yet: proto2, the `.proto` generator, well known types.

## 2026-11-06 unknown field preservation

- feature: a `derive Proto` struct may mark one `Bytes` field `proto "unknown"`; the reader keeps the raw bytes of every field it does not know (or whose wire type does not match) in it, and the writer appends them after the known fields, as Go's protobuf keeps and rewrites unknown fields.
## 2026-11-06 perf: derive Proto marshal writes into one buffer

- Kind: perf
- Commit: this commit
- Found by: the host 2 twin row (bench/results/std-twin-protobuf-2026-11-06.md): marshal 4.41x Go
- Test: testdata/corpus/protobuf (unchanged, byte-identical output), bench/perf/protobuf --ab marshal

The generated writers threaded an immutable Bytes through `protobuf.append_*`, each of which copied the whole buffer
(quadratic over a message). They now write into one BytesBuilder with `protobuf.put_*` (new, builder forms of the
append functions) and freeze once. `append_*` is unchanged.
Row after (host 2, n=5, Xo at 5e770dcbb): marshal 18.1 ms against Go 19.22 ms is 1.06x (1.026-1.172), was 4.41x; unmarshal 0.46x of Go.

## 2026-11-06 test: fresh seed fuzz of the wire decoder and derive Proto

- Kind: test
- Commit: this commit
- Found by: decision 0461 (a parser of untrusted input is Early until fuzzed)
- Test: internal/stdtest TestProtobufFuzz (XO_PROTOFUZZ_SEED, XO_PROTOFUZZ_BACKENDS), testdata/corpus/protobuf (seed 0)

`TestProtobufFuzz` makes the corpus generator's cases (random wire fields with wide varints, truncations and changed
copies; typed messages against dynamicpb including unknown field preservation) with a fresh seed and runs them on
arm64 and LLVM. Result: seed 2 on arm64: 8,942 generated cases pass; the committed seed 0 corpus is unchanged.
