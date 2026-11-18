# std/protoparse changes

## 2026-11-18: fix: protocompile's message and enum checks, range and option syntax

- Kind: fix
- Commit: this push
- Found by: task #152 (xw-1's protoparse fuzz against protocompile v0.14.1, seeds 5 to 37 at 5000 files: 1 or 2 of about 2550 rejected files accepted per failing seed)
- Go reference: protocompile v0.14.1 parser/validate.go validateMessage and validateEnum, parser/proto.y (ranges, option values, syntax, package, oneof, extend)
- Test: std/protoparse_test.xo "a message's reserved and extension ranges have no sign", "an option value is one identifier; syntax comes first; a package has no leading dot", "protocompile's validateMessage: extension ranges in proto3, reserved names, a field's name and number", "reserved names and numbers do not mix; a label before map; a oneof has a field and no empty statement", "protocompile's validateEnum: an enum has a value, none reserved", "an extend defines an extension" (accepted before)
- Shipped in: not yet released

parse now rejects what protocompile rejects in these files: a sign in a message's reserved or
extension range, a range whose end is below its start, names and numbers mixed in one reserved
statement, a string in extensions, a label before `map<`, an option value `true.` or `foo.bar`,
any statement (even `;`) before `syntax` or `edition`, `package .p`, an empty oneof or a `;` in
one, extension ranges in proto3, a reserved name that is not an identifier, a field or enum value
using a reserved name or number, two fields with one number, a field in an extension range, an
enum with no value, and an extend with no field. Each error gives protocompile's text at its
position, with the short scope names parse already used (`message M`, `enum E`).

## 2026-11-18: fix: allow_alias and message_set_wire_format take true or false; declared names are one identifier; no label in a oneof

- Kind: fix
- Commit: this push
- Found by: task #152 (xw-1's protoparse fuzz against protocompile v0.14.1, seed 4 at 5000 files on host 3: 3 of 2554 rejected files accepted: `option allow_alias = true0x10`, `rpc R11. (M5)`, `repeated f39 = 6` in a oneof)
- Go reference: protocompile v0.14.1 parser/validate.go validateEnum (allow_alias), parser/result.go (message_set_wire_format), parser/proto.y (names, oneof fields)
- Test: std/protoparse_test.xo "allow_alias and message_set_wire_format take true or false", "a declared name is one identifier", "a oneof field has no label" (accepted before)
- Shipped in: not yet released

parse accepted any value for allow_alias (a string, a number, `true0x10`); it now gives protocompile's
`enum E: expecting bool value for allow_alias option` at the value, and the same for message_set_wire_format.
A message, enum, value, field, group, oneof, service or rpc name with a `.` is
`syntax error: unexpected '.', expecting ...` at the dot, and `repeated`, `optional` or `required` in a
oneof is `syntax error: unexpected "repeated"`.

## 2026-11-17: fix: a proto3 enum whose first value is not zero is an error

- Kind: fix
- Commit: this push
- Found by: task #152 (xw-1's protoparse fuzz against protocompile v0.14.1, seed 3 at 5000 files on host 3: 1 of 2555 rejected files accepted, a proto3 enum with allow_alias starting at -1)
- Go reference: protocompile v0.14.1 parser/validate.go validateEnum (the proto3 first value check, before the alias checks)
- Test: std/protoparse_test.xo "a proto3 enum's first value is zero" (accepted before; top level, nested, with and without allow_alias; proto2 and editions still start anywhere)
- Shipped in: not yet released

parse rejected nothing for a proto3 enum's first value; it now gives protocompile's
`enum E: proto3 requires that first value of enum have numeric value zero` at the first value's number.

## 2026-11-17: feature: gen writes typed gRPC stubs for services

- Kind: feature
- Commit: this push
- Found by: task #236's map rows (grpc.ServiceDesc and five more; decision 0706)
- Go reference: grpc-go v1.79.2's protoc-gen-go-grpc output (the client interface, RegisterService)
- Test: cmd/xo TestProtoGenServiceRuns (a service with unary, server streaming, client streaming and bidirectional rpcs, generated, served and called); protoparse_test.xo "service stub names"
- Timing: none

A `service` block now generates a typed client (`SClient`, a method per rpc), typed stream wrappers and a
`handle_s_m` function per rpc that registers a typed handler on a grpc.Server; the file then uses std/grpc and
std/protobuf.

## 2026-11-17: fix: protoc's grammar for options, map keys, field numbers, groups and syntax; no syntax statement is proto2

- Kind: fix
- Commit: this push
- Found by: task #257 (xw-1's protoparse fuzz against protocompile v0.14.1, seed 2 at 5000 files: 55 files protocompile rejects that parse accepted, and 14 with no syntax statement read as proto3)
- Go reference: protocompile v0.14.1 parser/proto.y (optionName, constant, mapKeyType, the field number, group names), parser/lexer.go (numbers), parser/validate.go (syntax values); protoc reads a file with no syntax as proto2
- Test: std/protoparse_test.xo "a file with no syntax statement is proto2", "option names and values follow protoc's grammar", "map keys, field numbers, group names and numbers" (each case accepted before or read as proto3); all 55 rejected fuzz files give protocompile's line, column and text
- Shipped in: not yet released

Breaking: a .proto file with no `syntax` statement was read as proto3 and
is now proto2, as protoc reads it (its unlabeled fields are an error;
add `syntax = "proto3";`). `parse` also rejects, with protocompile's
text and position, an option name that is empty or not identifiers and
parenthesized names joined by dots, an option value that is not a
constant, a map key type other than an integer, bool or string, a
negative field number, a group name in lower case, a number with letters
run into it, and a syntax other than proto2 or proto3.

## 2026-11-17: fix: an empty statement in an extend block is an error

- Kind: fix
- Commit: this push
- Found by: xw-1's protoparse fuzz against protocompile v0.14.1, seed 4 (task #230 follow-up)
- Go reference: protocompile v0.14.1 parser/proto.y (extend bodies take fields and groups only: "syntax error: unexpected ';'")
- Test: std/protoparse_test.xo "an empty statement in an extend block is an error" (accepted before)
- Shipped in: not yet released

`parse` took a lone `;` inside `extend E { ... }` as it does in a message
body; protoc rejects it, and now so does `parse`.

## 2026-11-17: fix: four kinds of file protoc rejects are errors

- Kind: fix
- Commit: this push
- Found by: task #230 (xw-1's protoparse fuzz against bufbuild/protocompile v0.14.1, task #152: 9 of 74 rejected files accepted)
- Go reference: protocompile v0.14.1 parser/validate.go (validateMessage, validateEnum: tagRanges, aliases), parser/proto.y ("compact option must have a value")
- Test: std/protoparse_test.xo (one test per class, protocompile's texts; each failed before: parse accepted the file); the fuzz's nine files are rejected
- Shipped in: not yet released

`parse` accepted, and now rejects with protocompile's text: an enum with
`option allow_alias = true` and no two values sharing a number (and, the
other way, values sharing a number without it), overlapping `reserved`
ranges (messages and enums), overlapping `extensions` ranges and an
extension range overlapping a reserved one, and an option name followed
by another name with no `=` (`[import json_name = "j"]`). The scope names
the message or enum by its own name (protocompile's is fully qualified).

## 2026-11-17: feature: gen writes groups and DELIMITED message fields

- Kind: feature
- Commit: this push
- Found by: task #109 (protocompile's groups behavior)
- Go reference: protoc's proto2 groups and editions features.message_encoding (field and file level)
- Test: std/protoparse_test.xo ("gen writes a group and a DELIMITED message field")
- Shipped in: not yet released

`gen` no longer refuses a proto2 `group` or an editions `features.message_encoding = DELIMITED` message field: the field gets derive Proto's `group` option (`proto "N,group"`). A file level DELIMITED applies to message fields only, as protoc; a group in a oneof is still an error, since derive Proto writes a oneof variant length prefixed.

## 2026-11-16: fix: string literals whose escapes are not UTF-8 keep their bytes (breaking)

- Kind: fix
- Commit: this push
- Found by: owner decision #46 (task #109, protocompile's invalid_utf8_strings behavior)
- Go reference: protoc's tokenizer (ParseStringAppend keeps bytes), protocompile
- Test: std/protoparse_test.xo ("escapes giving invalid UTF-8 keep their bytes"); internal/stdtest/testdata/corpus/protoparse/aggregate_test.xo
- Shipped in: not yet released

Option values (`options: List[(Str, Bytes)]`) and `Field.default_value` are `Bytes`: `"\377"` is the byte
0xFF, not U+FFFD, and adjacent literals join before any read as text. `aggregate_pairs` and `aggregate_items` take
and give `Bytes`; a string `Token` keeps its bytes in `raw`. Code reading them as text calls `.to_str_lossy()`.

## 2026-11-16: fix: string escapes, dot floats, split type names, aggregate lists and extension keys, extend groups

- Kind: fix
- Commit: this push
- Found by: task #109, protocompile's internal/testdata files (desc_test1, desc_test_complex, desc_test_defaults)
- Go reference: protoc's tokenizer and parser
- Test: std/protoparse_test.xo (escapes, `.5`, `. google. protobuf .X`, aggregates, extend groups);
  scripts/protocdesc/testdata/split.proto and optvalues.proto through std/protodesc_test.xo
- Shipped in: not yet released

The lexer reads octal, hex, `\u`/`\U` and `\a \b \f \v \?` escapes (an octal `\001` read as `\0` then `01`) and a
number starting with a dot (`.1` failed); a type or package name may have its parts apart (`. google. protobuf .X`
failed); an aggregate option value keeps bracketed keys, lists and `< >` messages (they failed), and
`aggregate_items` keeps where each message starts; a group in an `extend` block keeps its message.

## 2026-11-16: feature: lex (protoc columns, comments); oneof options kept

- Kind: feature
- Commit: this push
- Found by: task #109 (protocompile source info)
- Go reference: protocompile's lexer (comment attribution), protoc's tokenizer columns
- Test: internal/stdtest/testdata/corpus/protoparse/lex_test.xo; std/protodesc_test.xo (source info built on it)
- Shipped in: not yet released

`lex` gives the tokens with protoc's columns (`pcol`, `pend`; a tab to the next multiple of 8) and the comments,
each with the token it leads or trails. `parse` keeps a oneof's options (`Message.oneof_options`) instead of
dropping them; the lexer also skips form feed and vertical tab as whitespace, as protoc does.

## 2026-11-16: feature: nested aggregate option values

- Kind: feature
- Commit: this push
- Found by: task #109 (protocompile custom options)
- Go reference: protocompile's aggregate option values
- Test: std/protodesc_test.xo, scripts/protocdesc/testdata/customopts.proto (`option (limits) = { ... }`)
- Shipped in: not yet released

An aggregate option value with a nested message (`{ a { b: 1 } }`) keeps its keys as dotted pairs (`a.b`) that
`aggregate_pairs` reads, instead of dropping the nested message as `{}`.

## 2026-11-16: feature: editions (2023, 2024); fix: gen refuses a group

- Kind: feature, fix
- Commit: this push
- Found by: task #109, the stdaudit gaps protocompile.IsEditionSupported and protocompile.behavior.editions;
  the group fix while mapping DELIMITED (derive Proto has no group encoding)
- Go reference: protoc 36.2 and bufbuild/protocompile v0.14.1 (editions grammar), descriptor.proto's FeatureSet
- Test: protoparse_test.xo "editions: ..." and "gen refuses a group and a DELIMITED message field..."; std/protodesc_test.xo's
  protoc cases gain scripts/protocdesc/testdata/editions2023.proto (byte exact with protoc)
- Shipped in: not yet released

`parse` reads `edition = "2023"` (and `"2024"`): `syntax` is `editions`, `edition` the year; `optional`, `required`,
`group` and string reserved names are errors there, reserved names are identifiers, and `features.NAME` options are
kept. `gen` follows `features.field_presence` (a singular field is an Option unless IMPLICIT or LEGACY_REQUIRED) and
`features.repeated_field_encoding` (EXPANDED is `unpacked`), the field's over the file's. Fix: `gen` refused nothing
for a proto2 group and wrote it as a length prefixed message, which protoc's readers do not take for a group; a group
and a DELIMITED field are now an error.

## 2026-11-16: feature: unpacked repeated output, imported enums and the well-known types

- Kind: feature
- Commit: this push
- Found by: task #109 (the protoparse coverage row's missing items; gotchas/derive-proto-across-files.md's
  open std half of decision 0683 item 4)
- Go reference: protoc's encoding of repeated fields (proto2 unpacked unless `[packed = true]`, proto3
  packed unless `[packed = false]`) and protoc-gen-go's mapping of google/protobuf's well-known types
- Test: protoparse_test.xo "gen writes proto2 repeated numbers, Bools and enums unpacked..." and "gen_set: an
  enum of an imported file resolves, the well-known types are std/protowkt's..." (the second was an
  is_err test before); cmd/xo TestProtoGenWellKnownEnumsUnpacked builds the output and runs a round trip
  (a proto2 `repeated int32` marshals as `08010802`, protoc's bytes)
- Shipped in: not yet released

`gen` writes a repeated number, Bool or enum field as `proto "N,unpacked"` in a proto2 file (unless
`[packed = true]`) and in proto3 with `[packed = false]`, so the generated message is byte identical to
protoc's for those fields. `gen_set` takes a field typed by an enum of another file of the set (derive Proto
takes it since decision 0683 item 4) and maps google/protobuf's well-known types to std/protowkt's
(`google.protobuf.Timestamp` is `protowkt.Timestamp`; the source then starts `use std/protowkt`); both were
errors.

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
