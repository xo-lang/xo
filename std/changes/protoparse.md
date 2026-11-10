# std/protoparse changes

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: std/protoparse_test.xo (`xo test --std-module=protoparse` on native and llvm); internal/stdtest corpus protoparse on go, arm64 and llvm

std/protoparse.xo now holds the bodies of std/native/protoparse.xo (removed), and the Go
backend compiles them from it as its own package. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-06: feature: std/protoparse, a .proto parser and Xo generator (prototype, branch protoparse-wip)

- Kind: feature
- Commit: this commit
- Found by: owner queue (Xo Lead): protobuf, then gRPC, then OTLP (decision 0530)
- Go reference: github.com/bufbuild/protocompile (descriptor shape only; not in the offline module cache)
- Test: std/native/protoparse_test.xo (9 tests: lexer, messages, enums, oneofs, maps, services, errors, generator output)

New module. `protoparse.tokens` lexes a proto3 file and `protoparse.parse` builds a `File` (messages, nested
messages, enums, oneofs, maps, services and rpcs, with positions in `ParseErr`). `protoparse.gen` turns a `File`
into Xo source with `derive Proto` structs. Not yet: imports resolved across files, options beyond
`packed`, fuzzing, a timing row, the `xo proto gen` command.

## 2026-11-07: feature: proto2 syntax (labels, required, default, groups)

- Kind: feature
- Commit: this commit
- Found by: owner queue (Xo Lead): protoparse proto2
- Go reference: github.com/bufbuild/protocompile (descriptor shape only)
- Test: std/native/protoparse_test.xo (proto2 labels, required, default, groups, extensions, unlabeled field)

`protoparse.parse` reads a `syntax = "proto2"` file as proto2 instead of as proto3: every field outside a oneof needs
`required`, `optional` or `repeated`, `required` sets `Field.required`, and a `group` becomes a nested message plus
a lower case field of that type. `default` and extension ranges are dropped like other options. A proto3 file still
rejects `required` and `group`. Known gap: `derive Proto` writes proto2 repeated numeric fields packed (the reader
accepts both), where protoc's proto2 output is unpacked.
