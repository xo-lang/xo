# std/grpcreflect changes

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
