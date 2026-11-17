# std/grpcreflect changes

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
