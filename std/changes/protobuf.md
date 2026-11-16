# std/protobuf changes


## 2026-11-16: feature: protobuf.size and protobuf.merge_from (proto.Size, proto.Merge)

- Kind: feature
- Commit: this push
- Found by: task #102 (std/protobuf features): stdaudit gaps proto.Size, MarshalOptions.Size, proto.Merge
- Go reference: google.golang.org/protobuf v1.36.12 proto/size.go, proto/merge.go
- Test: golden proto_size_merge (go, arm64, LLVM): size equals marshal's length; merge_from with scalars, a zero scalar, a nested message, a list and a map
- Timing: none (size is marshal's own sizes pass)

`protobuf.size(v)` is the length `marshal(v)` writes, from the derive's sizes
pass without writing bytes. `protobuf.merge_from(dst, src)` returns dst with
src's populated fields merged in by proto.Merge's rules.

## 2026-11-15: feature: field number range (protowire's MinValidNumber, MaxValidNumber, reserved range, Number.IsValid)

- Kind: feature
- Commit: this push
- Found by: scripts/stdaudit gaps protowire.MinValidNumber, MaxValidNumber, FirstReservedNumber, LastReservedNumber and Number.IsValid (decision 0655)
- Go reference: google.golang.org/protobuf v1.36.12 encoding/protowire wire.go (Copyright 2018 The Go Authors, BSD-style license)
- Test: std/native/protobuf_test.xo "valid_number" at every boundary (arm64, llvm; corpus protobuf on go)
- Shipped in: not yet released

`protobuf.valid_number(num)` and the four constants, as protowire declares them.

## 2026-11-15: feature: `ProtoErr` is an open enum (decision 0649)

- Kind: feature
- Commit: this commit
- Found by: decision 0649 (a new variant of a std error enum broke two SDKs, 0644's `ClientErr.Tls`)
- Go reference: none (Go's errors are values compared with errors.Is; Rust's `#[non_exhaustive]`)
- Test: compiler/check's open enum test; the open_enum golden; xo check over the programs that use the module

`ProtoErr` is declared `open`: a `match` on it outside std needs a `_` (or
binding) arm, warning XO0306 in 0.52 and an error after, so that std can
add a variant in a minor release without breaking a build. Matches that
already have `_` are unchanged.

## 2026-11-12: perf: derive Proto writes each field's key with its value

- Kind: perf, feature (put_key_varint, put_key_fixed32, put_key_bytes)
- Commit: this push
- Found by: XO_PROF=cpu of bench/perf/protobuf marshal on linux/arm64 host 1 after the word writers: the tag pushed alone, a byte at a time (cabi.obj_rs_push_byte 30% cumulative with obj.reserve)
- Go reference: protobuf-go's generated fast path appends a field's precomputed tag bytes and its value in one step
- Timing: bench/results/otlp-derive-2026-11-11.md sixth table: protobuf marshal 0.847, otlp traces 0.923, metrics 0.969 of main (linux/arm64 host 1, --ab)
- Test: protobuf corpus "the put_key_ forms write the tag then the value" (fields 1 to 536870911, wire 0, 2, 5, values each side of 2^49); the protobuf, protowkt, protojson, protodesc, protoparse, textproto and otlp corpora on go, arm64, llvm; goldens otlp_export, derive_empty_struct
- Shipped in: not yet released

`put_key_varint(out, key, v)`, `put_key_fixed32` and `put_key_bytes` take a field's key (`num << 3 | wire`) and
write it and the value (or the length prefix) together, as one `BytesBuilder.write_le` when the key is one byte and
the value fits beside it. The generated writers call them for every field, and a nested message's
`proto_kwrite__T` writes its key with its length, where each field pushed its tag alone. Output bytes are unchanged.

## 2026-11-12: perf: varints and fixed width values are written as one word

- Kind: perf
- Commit: this push
- Found by: XO_PROF=cpu of bench/perf/protobuf marshal on linux/arm64 host 1 (put_varint 43% cumulative, obj_rs_push_byte and obj.reserve per byte; the size pass only 12.7%)
- Go reference: protowire.AppendVarint and AppendFixed64 append each value with one append
- Timing: bench/results/otlp-derive-2026-11-11.md fifth table: protobuf marshal 0.856, otlp traces 0.972, metrics 0.916 of main (linux/arm64 host 1, --ab)
- Test: the protobuf (put_ forms against append_ forms, protowire's cases), protowkt, protojson, protodesc, protoparse, textproto and otlp corpora on go, arm64, llvm; goldens otlp_export, derive_empty_struct
- Shipped in: not yet released

`put_varint` gathers a varint of up to 8 bytes into one word and appends it with `BytesBuilder.write_le`
(a longer one writes its last bytes after); `put_fixed32` and `put_fixed64` are one `write_le` each. They pushed
a byte at a time. Output bytes are unchanged.

## 2026-11-11: perf: derive Proto computes each nested message's size once

- Kind: perf
- Commit: this push
- Found by: XO_PROF=cpu after the size pass (its size functions were about a third of otlp traces, recomputed per level)
- Go reference: google.golang.org/protobuf caches a message's size between Size and the append (XXX_sizecache)
- Timing: bench/results/otlp-derive-2026-11-11.md fourth table: protobuf marshal 0.851, otlp traces 0.856, metrics 0.897 of the size pass (linux/arm64 host 1, --ab)
- Test: the protobuf, protowkt, protojson and otlp corpora on go, arm64, llvm; derive goldens
- Shipped in: not yet released

The size pass records nested message sizes in order (`proto_sizes__T`) and the writer reads them back; output bytes
are unchanged.

## 2026-11-11: perf: derive Proto marshals in one buffer after a size pass

- Kind: perf
- Commit: this push
- Found by: decision 0549 item 3 (otlp metrics as derive structs measured 1.205 of the hand writer; the profile showed a Bytes per nested message)
- Go reference: google.golang.org/protobuf's marshal (Size, then one append buffer)
- Timing: bench/results/otlp-derive-2026-11-11.md third table: bench/perf/protobuf marshal 0.798, otlp traces 0.776 (linux/arm64 host 1, --ab)
- Test: the protobuf, protowkt, protojson and otlp corpora (protobuf-go's bytes) on go, arm64, llvm; derive goldens
- Shipped in: not yet released

The generated `proto_marshal__T` now computes the message size (`proto_size__T`) and writes every nested message,
packed list and map entry into one `BytesBuilder` of that capacity (`proto_write__T`), where each was built as a
`Bytes` of its own and copied into its parent. Output bytes are unchanged.

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
