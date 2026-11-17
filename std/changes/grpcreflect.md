# std/grpcreflect changes



## 2026-11-17: perf: each file is encoded once, when the service is made

- Kind: perf
- Commit: this commit
- Found by: task #273, the stress evidence run (memory grew 13% over 30 minutes: every answer re-encoded its files and rebuilt every file's symbol set)
- Go reference: grpc-go's reflection server marshals on each ask; Xo keeps the bytes, as answers never change
- Test: std/grpcreflect "answers carry each file's encoding, the file found even when two share a name"
- Shipped in: not yet released

## 2026-11-17: fix: a field number above 536870911 fails a request or response, as protobuf-go

- Kind: fix
- Commit: this commit
- Found by: TestGrpcReflectFuzz on the Mac (task #273): a request with an unknown field numbered 1,168,186,489 was read as empty; protobuf-go fails it
- Go reference: google.golang.org/protobuf v1.36.12 internal/impl/decode.go (a tag's number above protowire.MaxValidNumber is errDecode)
- Test: std/grpcreflect_test.xo decode_request of that request fails; field 536870911 is still skipped
- Shipped in: not yet released
## 2026-11-17: fix: a packed extension number cut short is an error

- Kind: fix
- Commit: this commit
- Found by: task #273, TestGrpcReflectFuzz seed 2 (decode_answer read a response whose packed extension_number ended in a varint cut short as no numbers; protobuf-go refuses it)
- Go reference: google.golang.org/protobuf v1.36.12 encoding/protowire ConsumeVarint (errCodeTruncated, errCodeOverflow)
- Test: std/grpcreflect_test.xo "repeated answers merge and every string is checked, as protobuf-go reads them" (the cut short response decoded Ok before)
- Timing: none
- Shipped in: not yet released
## 2026-11-17: fix: requests and responses read as protobuf-go reads them

- Kind: fix
- Commit: this commit
- Found by: TestGrpcReflectFuzz (task #273), the new differential fuzz against protobuf-go reading grpc-go's reflection messages
- Go reference: google.golang.org/protobuf v1.36.12 proto.Unmarshal (a repeated message field merges; strings are checked as UTF-8)
- Test: std/grpcreflect_test.xo "repeated answers merge and every string is checked" (each case read differently before), arm64 and LLVM; TestGrpcReflectFuzz 2,000 requests and 2,000 responses
- Behavior: a repeated ExtensionRequest or answer block merges into the first (lists append, the last scalar wins) instead of replacing it; a list_services text, a response's host and its original request that protobuf-go refuses are refused
- Timing: none
- Shipped in: not yet released

## 2026-11-17: fix: files that import each other no longer loop the file walk

- Kind: fix
- Commit: this commit
- Found by: task #273's review (a FileDesc built by hand or decoded can import in a cycle, which a .proto parser rejects)
- Go reference: google.golang.org/grpc v1.79.2 reflection/internal/internal.go FileDescWithDependencies (its registry has no cycles)
- Test: std/grpcreflect_test.xo "a cycle of imports ends the file walk, each file sent once" (looped for good before), arm64 and LLVM
- Timing: none (a set of the files walked)
- Shipped in: not yet released

## 2026-11-17: feature: gRPC server reflection (grpc.reflection.v1 and v1alpha)

- Kind: feature
- Commit: this commit
- Found by: task #236 (std Proven; scripts/stdaudit/map.txt grpc.behavior.reflection)
- Go reference: google.golang.org/grpc v1.79.2 reflection/serverreflection.go and reflection/internal/internal.go
  (FileDescWithDependencies, the not found errors, sorted services and extension numbers)
- Test: std/grpcreflect_test.xo (answers, wire round trips, a server over the wire, arm64 and LLVM);
  TestGrpcReflectInterop (Xo and grpc-go clients against Xo and grpc-go servers print the same answers); golden
  grpcreflect_calls
- Timing: none (a new module)
- Shipped in: not yet released
