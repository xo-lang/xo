# std/protodesc changes

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
