# test/protoconf

Testee of Google's protobuf conformance runner (v29.3) over std/protobuf and std/protojson (ADR 0546 item 7), and
`gotestee/`, the same envelope handling over protobuf-go's protojson and dynamicpb, so the two pass counts compare the
JSON mapping libraries. Both read a FileDescriptorSet as their first argument:

    protoc -I src/src -I src --include_imports --descriptor_set_out=set.bin src/src/google/protobuf/test_messages_proto3.proto src/src/google/protobuf/test_messages_proto2.proto
    xo build -o conformance_xo ./test/protoconf
    conformance_test_runner --maximum_edition 2023 --failure_list expected_failures.txt ./run.sh   # run.sh: exec conformance_xo set.bin

Results: bench/results/protobuf-conformance-2026-11-08.md (kept runner build: infrastructure/offload.md). Not part of any test tier: the runner is built by hand (cmake, abseil,
jsoncpp) on an offload host.
