# std/protodesc changes

## 2026-11-17: fix: a name declared twice is an error

- Kind: fix
- Commit: this push
- Found by: task #285: xo-4c's grpcreflect stress files (two `extend` blocks declaring `x4` in package p4) built
  without an error; protoc and protobuf-go's protodesc.NewFile reject them
- Go reference: protobuf-go reflect/protodesc/desc_init.go makeBase ("descriptor %q already declared")
- Test: std/protodesc_test.xo ("build: a name declared twice is an error, as protobuf-go's NewFile (desc_init.go
  makeBase)", fails before: build returned Ok)
- Shipped in: not yet released

`build` and `build_with_source_info` return an error (line 0) for a full name the file declares twice: two
extensions, a message and an enum value (enum values are in the enum's parent scope), two services, a field and a
nested message. The names are checked in protobuf-go's order (enums and values, messages with their fields,
oneofs, enums, nested messages and extensions, extensions, services and methods), so the first duplicate named is
the one protobuf-go names.

## 2026-11-17: fix: only a singular string field's last value has to be UTF-8

- Kind: fix
- Commit: this push
- Found by: task #217 (fuzz evidence for Proven): the protodesc corpus fuzz, seed 2, gave a descriptor protobuf-go
  accepts and `decode` rejected
- Go reference: protobuf-go's descriptorpb decoding (a repeated singular field keeps its last value)
- Test: std/protodesc_test.xo ("a singular string field's last value must be UTF-8, not an earlier one", "the fuzz
  input of seed 2 that protobuf-go accepts decodes", both fail before)
- Shipped in: not yet released

`decode` failed with `Utf8` when an earlier occurrence of a singular string field (a name, a package, a type name)
held bytes that are not UTF-8, though a later occurrence replaced it; it now checks only the value kept.

## 2026-11-16: fix: string literals whose escapes are not UTF-8 keep their bytes (breaking)

- Kind: fix
- Commit: this push
- Found by: owner decision #46 (task #109, protocompile's invalid_utf8_strings behavior)
- Go reference: protoc's tokenizer (ParseStringAppend keeps bytes), protocompile
- Test: std/protodesc_test.xo ("build: escapes giving invalid UTF-8 keep their bytes", fails before)
- Shipped in: not yet released

`build` writes a bytes default (`"\377"` as `\377`, protoc's CEscape) and a string or bytes option value,
built in or custom, with the literal's bytes; they were read through U+FFFD.

## 2026-11-16: fix: a proto3_optional written false is written back

- Kind: fix
- Commit: this push
- Found by: task #27 (fuzz evidence for Proven): the new fuzz part of the protodesc corpus (random FileDescriptorProtos
  from protobuf-go), seed 0, failed to round trip
- Go reference: google.golang.org/protobuf v1.36.12 (descriptorpb keeps a proto2 bool's presence)
- Test: std/protodesc_test.xo "proto3_optional written false is written back"; corpus protodesc's FUZZ and MUTATED
  rows (120 random descriptors and 120 changed ones, arm64, llvm and go); internal/stdtest TestProtodescFuzz
  (XO_PROTODESCFUZZ_SEED) for fresh seeds
- Timing: none (one more Bool per field)
- Shipped in: not yet released

`decode` keeps whether `proto3_optional` was on the wire (`FieldDesc.proto3_optional_set`), so `encode` writes back
a `proto3_optional: false` protobuf-go wrote, as it does `json_name`; before, those 3 bytes were dropped. A string
field that is not UTF-8 is still refused (an Xo `Str` is UTF-8), where protobuf-go accepts any bytes in
descriptor.proto's proto2 strings.

## 2026-11-16: feature: build_with_source_info's extra comments and extra option locations modes

- Kind: feature
- Commit: this push
- Found by: task #109, the stdaudit gaps protocompile.SourceInfoExtraComments and SourceInfoExtraOptionLocations
- Go reference: github.com/bufbuild/protocompile v0.14.1 sourceinfo (WithExtraComments, WithExtraOptionLocations)
- Test: std/protodesc_test.xo "build_with_source_info's extra modes give protocompile's SourceCodeInfo" (each mode
  and both, byte exact); by hand, protocompile's internal/testdata files and the protoc cases in every mode
- Shipped in: not yet released

`build_with_source_info(src, name, deps, extra_comments: true, extra_option_locations: true)`: with
`extra_comments` every location (not only a declaration's) takes the comments around it, and a comment on the line
of a closing symbol goes to the token before it; with `extra_option_locations` an option's message literal has a
location for each field it sets and each element of a list, with the path into the options message, as protocompile
writes them.

## 2026-11-16: fix: option values, number and bytes defaults, json_name, groups in extend blocks

- Kind: fix
- Commit: this push
- Found by: task #109, protocompile's internal/testdata files (desc_test_complex, desc_test_defaults, desc_test2,
  desc_test_proto3_optional) built and compared with protoc
- Go reference: protoc 36.2's descriptors of those files
- Test: std/protodesc_test.xo, the protoc case scripts/protocdesc/testdata/optvalues.proto (byte exact, with source
  info) and split.proto
- Shipped in: not yet released

`build` writes each option of a repeated message extension as its own element (they were merged), each `{ ... }` of a
repeated message field in an option value apart, an extension key in an option value (`[pkg.ext]`, and
`(ext).(pkg.ext)` names), and a map entry's unset key and value; number defaults as protoc writes them (`0x10` is
`16`, `.25` is `0.25`, a float with 6 significant digits); bytes defaults C escaped; `json_name` also when empty; an
extension's `proto3_optional` in a proto3 file; and a group declared in an `extend` block (its message and the
TYPE_GROUP field), which failed with an unknown type.

## 2026-11-16: feature: build_with_source_info; fix: custom options on oneofs and extension ranges

- Kind: feature, fix
- Commit: this push
- Found by: task #109 (protocompile.SourceInfoStandard)
- Go reference: protoc 36.2 --include_source_info; protocompile's sourceinfo (standard mode)
- Test: std/protodesc_test.xo, protoc's SourceCodeInfo for the 47 cases of scripts/protocdesc outside
  google/protobuf (byte exact; scripts/protocdesc/testdata/sci.proto and sci3.proto for comment placement, groups,
  extend, oneof options, ranges, streams); internal/stdtest/testdata/corpus/protodesc/sourceinfo_test.xo
- Shipped in: not yet released

`build_with_source_info(src, name, deps)` parses and builds a file with `source_code_info` as protoc writes it: the
locations of the file, its declarations and their parts, with leading, trailing and detached comments. `build` now
writes custom options set on a oneof or an extension range (protoc does; they were left out).

## 2026-11-16: feature: custom options

- Kind: feature
- Commit: this push
- Found by: task #109 (protocompile custom options)
- Go reference: protoc 36.2's FileDescriptorProto for a file with custom options
- Test: std/protodesc_test.xo, the protoc case scripts/protocdesc/testdata/customopts.proto (byte exact)
- Shipped in: not yet released

`build` writes custom options (`(ext)` and `(ext).a.b`, extensions of the options messages in the file or an import)
as protoc does: after the known options in extension number order, with every scalar kind, enum names, and message
values from aggregates or several options merged into one message.

## 2026-11-16: feature: editions descriptors

- Kind: feature
- Commit: this push
- Found by: task #109 (protocompile.IsEditionSupported)
- Go reference: protoc 36.2's FileDescriptorProto for an edition 2023 file
- Test: std/protodesc_test.xo, the protoc case scripts/protocdesc/testdata/editions2023.proto (byte exact)
- Shipped in: not yet released

`build` of an editions file writes syntax `editions`, the edition's number (EDITION_2023, 1000), and each
`features.NAME` option as a FeatureSet in the file's, message's, field's, enum's, value's, service's or method's
options, as protoc does.

## 2026-11-16: feature: build, descriptors from std/protoparse as protoc writes them

- Kind: feature
- Commit: this push
- Found by: task #109 (the partial Go row of std/protodesc: building them from protoparse)
- Go reference: protoc 36.2's descriptor builder (`--descriptor_set_out`); bufbuild/protocompile's shape
- Test: std/protodesc_test.xo, 47 files byte for byte against protoc (the protoparse corpus files protoc compiles, descriptor.proto and the well known types, proto2 and proto3 feature files; scripts/protocdesc), arm64 and LLVM
- Timing: none (one pass over the parsed file; a name lookup scans the symbols for a package prefix)

`build(f, name, deps)` gives the FileDescriptorProto protoc writes for a file std/protoparse parsed: names resolved by
protobuf's scoping, map entry messages, proto3 optional's synthetic oneofs, json_name, groups, defaults, extend blocks,
reserved and extension ranges, and descriptor.proto's own options (custom and source retention options left out).

## 2026-11-07: port: std/protodesc, descriptor.proto as Xo values

- Kind: port
- Commit: this push
- Found by: ADR 0546 (protobuf and gRPC to full coverage), items 1 and 2
- Go reference: google.golang.org/protobuf v1.36.12 (descriptorpb, protodesc, the known types)
- Test: see below
- Timing: not measured (a codec for reflection and ProtoJSON, not a hot path yet)
- Shipped in: not yet released

`protodesc.decode` and `encode` (spec/std.md 72). Hand written codec over std/protobuf (derive Proto does not expand in std, decision 0549). Test: internal/stdtest/testdata/corpus/protodesc (descriptor.proto, the well known types, grpc-go health, reflection and test descriptors: decode, counts, byte exact re-encode) on arm64, LLVM and the Go backend; `xo test --std-module=native/protodesc`.
