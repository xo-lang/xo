# std/protoparse changes

## 2026-11-16: fix: generated code compiles; a type of an imported file resolves

- Kind: fix
- Commit: this push
- Found by: task #102 (stdaudit gap proto.behavior.proto_files): building `xo proto gen` output for an import failed, and so did a single file's
- Go reference: protoc's name resolution (descriptor.proto scoping, `import public`)
- Test: cmd/xo TestProtoGenBuilds (two files, one importing the other, generated into one directory with a main, run; failed before with XO0110 and "unknown type"); protoparse tests for gen_set, `import public`, an imported enum and a type outside the set

`gen` wrote `use std/protobuf`, which `derive Proto` does not need, so the
output failed to build with XO0110 (unused import); it is gone. Types and
fields are now `pub`, so the code `derive Proto` writes for a message is
visible from the other generated files and from other packages. The new
`gen_set(files)` generates a set of files, a type resolving in its own
package's scopes, then in the files it imports and their `import public`
files; `xo proto gen` parses every file it is given first and uses it.
`File.public_imports` lists the public imports. A field typed by an enum of
another file, or by a type outside the set (the google/protobuf well-known
types), is an error: `derive Proto` takes only an enum of the same file and
a struct of the same package.

## 2026-11-16: feature and fix: options, ranges and extend kept; a proto2 map field needs no label

- Kind: feature, fix
- Commit: this push
- Found by: task #109 (std/protodesc.build against protoc): protoc accepts `map<K, V> f = 1;` in proto2, which parse refused (`proto2 field needs required, optional or repeated`)
- Go reference: protoc 36.2's parser
- Test: std/protodesc_test.xo's protoc cases (features2.proto has a proto2 map field, which failed before), std/protoparse_test.xo, arm64 and LLVM
- Timing: none

`parse` keeps what it read and dropped: options as (name, value) pairs on the file, messages, fields, enums, values,
services and rpcs (flat aggregates as pairs, `aggregate_pairs`), a field's `default` and `json_name`, `reserved` and
`extensions` ranges (with their options) and names, `extend` blocks (`Extend`), public and weak imports, whether a
group field is a group, where each map sits among the nested messages, and whether an rpc has a body. The new fields
have defaults, so code building these structs is unchanged. A proto2 `map` field no longer needs a label.

## 2026-11-10: fix: `ParseErr` displays as `LINE:COL: MSG` again

- Kind: fix
- Commit: this commit
- Found by: cmd/xo TestProtoGen on main after the 0588 merge (Lang 2, lead)
- Go reference: none
- Test: cmd/xo TestProtoGen (`xo proto gen` on a bad file names `bad.proto:2:`)

The 0588 merge kept the declaration's `derive Display` and the native
`parseerr_display` that used to override it, so the derive won and
`xo proto gen` printed `ParseErr{line: 2, ...}` instead of `2:13: MSG`.
ParseErr now has its own `display()` method.

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
