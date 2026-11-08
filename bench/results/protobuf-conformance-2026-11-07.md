# protobuf conformance, std/protojson against protobuf-go (2026-11-07)

Runner: Google's conformance_test_runner of protobuf v29.3 (cmake build, abseil 20240116.0, jsoncpp 1.9.6), linux/arm64
offload host 2, `--maximum_edition 2023`. Schema: test_messages_proto3.proto as a FileDescriptorSet from the runner's protoc.
Both testees (test/protoconf) share one envelope handler; they differ only in the library that converts between wire bytes
and JSON (std/protojson against protobuf-go v1.36.12 protojson and dynamicpb).

| testee | successes | skipped | expected failures | unexpected failures |
| --- | --- | --- | --- | --- |
| std/protojson (Xo) | 1411 | 3951 | 2 (listed) | 0 |
| protobuf-go v1.36.12 | 1412 | 3951 | 0 | 0 |

The 2 listed failures (test/protoconf/expected_failures.txt) are `Required.Proto3.ProtobufInput.UnknownVarint.ProtobufOutput`
and `...UnknownOrdering.ProtobufOutput`: the Xo testee re-serializes protobuf input through JSON, which drops unknown fields,
where Go's proto.Message keeps them. They test the testee's binary round trip, not the JSON mapping. One more test appears in Go's
log only (113 differ in the logs before the re-serialization change, 1 after); not yet identified.

The suite also found one protojson bug that the Go differential corpus had not: the zigzag of sint64's minimum and maximum
faulted (`n << 1` overflow); fixed, with corpus rows.

## Gaps (what the runner skipped)

- **Text format**: 3951 tests skipped (the testee answers `skipped`); Xo has no protobuf text format.
- **JSPB**: skipped in the same count; not planned.
- **proto2 message types**: 843 tests skipped (TestAllTypesProto2 and the editions messages); std/protojson reads proto2 descriptors, but
  the testee handles only the proto3 message. A real user gap: proto2 support in the testee, then the failures it shows.
- Unknown field preservation across protobuf round trips (the 2 listed failures).
