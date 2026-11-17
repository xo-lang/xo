# std/grpcreflect feature audit (decision 0691)

Every feature spec/std.md section 90 promises for std/grpcreflect, each with the test that holds it. The module
ports grpc-go v1.79.2's `reflection` package (serverreflection.go, internal/internal.go); TestGrpcReflectInterop
crosses Xo's server and client with grpc-go's in all four pairings. Written 2026-11-17 (xo-4c, task #273).

- [x] register: ServerReflectionInfo served as v1 and v1alpha on a grpc.Server | task #236 (spec/std.md 90) | internal/stdtest/grpcinterop_test.go:TestGrpcReflectInterop
- [x] list_services: the services registered before it and itself, sorted | spec/std.md 90 | internal/llvm/testdata/golden/grpcreflect_calls
- [x] file_by_filename: the file and its imports breadth first, the first always, the others once per stream | spec/std.md 90 | std/grpcreflect_test.xo
- [x] file_containing_symbol: message, nested message, enum, enum value, field, oneof, extension, service, method | spec/std.md 90 | std/grpcreflect_test.xo
- [x] file_containing_extension and all_extension_numbers_of_type (sorted) | spec/std.md 90 | internal/stdtest/grpcinterop_test.go:TestGrpcReflectInterop
- [x] not found: error code 5 "proto: not found" for an unknown name, file, extension or type | spec/std.md 90 | std/grpcreflect_test.xo
- [x] an empty request ends the stream InvalidArgument "invalid MessageRequest: <nil>" | spec/std.md 90 | std/grpcreflect_test.xo
- [x] the host as valid_host and the request as original_request in each response | spec/std.md 90 | std/grpcreflect_test.xo
- [x] request and response messages round trip their wire form | spec/std.md 90 | std/grpcreflect_test.xo
- [x] requests and responses read as protobuf-go reads them (merged repeated fields, UTF-8 checked) | spec/std.md 90 | internal/stdtest/grpcreflectfuzz_test.go:TestGrpcReflectFuzz
- [x] a cycle of imports among given descriptors ends the file walk | spec/std.md 90 | std/grpcreflect_test.xo
- [x] client: ask, list_services, file_by_filename, file_containing_symbol against grpc-go's server | spec/std.md 90 | internal/stdtest/grpcinterop_test.go:TestGrpcReflectInterop
