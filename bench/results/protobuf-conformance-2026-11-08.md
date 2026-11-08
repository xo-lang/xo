# protobuf conformance with proto2, std/protojson against protobuf-go (2026-11-08)

Runner: Google's conformance_test_runner of protobuf v29.3, linux/arm64 offload host 2, `--maximum_edition 2023`. Schema: a
FileDescriptorSet from the runner's protoc holding test_messages_proto3.proto and test_messages_proto2.proto (set2.bin). Both
testees (test/protoconf) share one envelope handler; see [2026-11-07](protobuf-conformance-2026-11-07.md) for the setup.
Revision c6e81c6b7 (std/native/protojson.xo with the required-field checks).

| testee | successes | skipped | expected failures | unexpected failures |
| --- | --- | --- | --- | --- |
| std/protojson (Xo) | 2670 | 2689 | 4 (listed) | 0 |
| protobuf-go v1.36.12 | 2675 | 2689 | 0 | 0 |

Against 2026-11-07 (1411 and 1412 successes) the proto2 message type adds 1259 passing tests; the runner's skipped count falls from
3951 to 2689 (text format and JSPB). The editions message types (843 tests) are still answered `skipped`.

The 4 listed failures (test/protoconf/expected_failures.txt) are the unknown field round trips `UnknownVarint` and `UnknownOrdering`
with protobuf output, for proto3 and proto2: the Xo testee re-serializes through JSON, which keeps no unknown fields. The Go testee
passes them (the runner reports its 4 listed names as stale, "errors: 4"). Go's testee drops the proto2 MessageSet messages and
their extensions from its descriptors (protobuf-go refuses them), so those cases are not compared against Go.

Fixed on the way: proto2 required fields now fail a parse when missing in `msg_json` and `msg_wire_obj`, with corpus rows from Go's
protojson (scripts/stdcorpus/protojsonoracle, pjtest2.proto).

## Update: unknown fields kept (later 2026-11-08)

The testee's protobuf-in, protobuf-out path now calls `protojson.normalize` (wire bytes to wire bytes as protobuf-go's
Unmarshal then Marshal: known fields canonical, unknown fields kept) instead of marshal then unmarshal through JSON.
Runner rebuilt in /work/conf on host 2 (see offload.md "Kept directories"), set.bin from the same two descriptor files.

| testee | successes | skipped | expected failures | unexpected failures |
| --- | --- | --- | --- | --- |
| std/protojson (Xo) | 2674 | 2689 | 0 | 0 |

The 4 listed failures all pass (the runner flags them as stale until they are removed), and test/protoconf/expected_failures.txt is
now empty. The runner's second pass (text format) skips all 843 of its tests, as before.

## Gaps

- Text format and JSPB: skipped, as before.
- Editions message types: 843 tests skipped; next step is the editions descriptors in the set.
