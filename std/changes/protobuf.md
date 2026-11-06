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
