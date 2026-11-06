# 0530: std/protobuf, derive Proto and a .proto generator

Status: Proposed (Xo Lang 3, 2026-11-06; the owner accepts the API)

## Context

Xo has no protobuf. The owner approved (2026-11-06, relayed by the Lead) three std modules in order: protobuf, then gRPC over
std/http's HTTP/2 (unary and the three streaming modes, status codes, metadata, deadlines through task cancellation; the
reference is grpc-go with interop against a Go server and client), then OTLP export for std/trace and metrics (needs both).
The reference for protobuf is google.golang.org/protobuf (`proto.Marshal`, `proto.Unmarshal`, `protojson` later): the
output must be wire-identical to it. `Proto` is already a derivable name (spec/core.md 2.9, "planned after 1.0", currently
a Serial placeholder with no codec) and `std/prelude.xo` has `interface Proto {}`.

Every typed format so far is a derive that generates ordinary Xo source per type (`derive Json`, `Xml`, `Yaml`, `Cbor`), with
a per-format field tag and a small std module for the dynamic tree and the helpers the generated code calls. Protobuf is
different in one respect: its schema lives in a `.proto` file that other systems share, so the Xo types should usually be
generated from it, and the field numbers (not names) are the wire identity.

## Decision (proposed)

1. **std/protobuf**: the wire layer, native, with Go's `protowire` as its reference: `Encoder`/`Decoder` over varint,
   fixed32, fixed64, length-delimited and group wire types, zigzag, tags, size computation, and an `Unknown` field set
   (kept and re-emitted in order, as Go does). Malformed input is a `ProtoErr` with Go's cases (truncated, overflow, bad
   wire type, bad UTF-8 in a `string` field, recursion limit).
2. **derive Proto** on structs (and enums for oneofs and enum values), generating `proto_marshal__T`, `proto_unmarshal__T`
   and `proto_size__T` as Xo source like `derive Cbor`; `protobuf.marshal[T]`, `protobuf.unmarshal[T]` are rewritten to them.
   A field's number is its tag: `name proto "1": Int`, options after a comma (`packed`, `zigzag`, `fixed`, `oneof=group`).
   Proto3 semantics first (implicit presence for scalars, `optional` as Option, `repeated` as List, packed by default,
   `map<K,V>` as Map, a oneof as an enum field, a nested message as a struct, Timestamp/Duration as Time/Duration, Any,
   Struct/Value later). Proto2 (required, defaults, extensions, groups) after proto3 is Solid.
3. **A generator**: `xo proto gen FILE.proto --out DIR` parses a `.proto` (proto3 and proto2 syntax, imports, nested
   types, services as interfaces for the gRPC step) and writes `.xo` structs with `derive Proto`. The parser is its own
   Xo module (std/protoparse), fuzzed. No `protoc` dependency: Go's `protocompile` (a pure Go parser) is the differential
   oracle for the parse tree and for descriptors.
4. **Wire identity**: marshal writes fields in field-number order (Go's default for non-map fields), with `deterministic`
   sorting map keys as `proto.MarshalOptions{Deterministic: true}`. The differential oracle is internal/stdtest/protoref
   (its own Go module, like cborref), driving Go's `dynamicpb` from descriptors so no generated Go code is needed; the
   corpus generator is scripts/stdcorpus/protobuf.go.
5. **Maturity** (decision 0461): lands at least Solid on all three axes: Go's own tests ported (protowire, a slice of
   internal/impl and proto's encode/decode tests) or a Go differential, the parsers fuzzed (wire decoder and .proto
   parser), and a timing row against Go's package.

## Consequences

- A new std module and a new subcommand; `Proto` stops being a placeholder in the derivable table (`kind` Marked, `format`
  "proto"; `proto` joins the field format words in the parser).
- The parser gains the `proto` format word; `XO0422` and `XO0425` extend as for Cbor.
- gRPC (the next ADR) builds on `protobuf.marshal`/`unmarshal` and the generator's service output; OTLP on both.

## Alternatives

- Reflection over descriptors only (no derive): wire-correct and simple, but untyped and slower; kept as `protobuf.Dynamic`
  later for tools, not the main API.
- Require `protoc` plus a plugin: rejected, a build dependency Xo's single toolchain avoids.
- Names instead of numbers in the tag: rejected, the number is the wire identity and the `.proto` source of truth.

## Open questions for the owner

- Tag syntax (`name proto "1,packed"` as proposed, or a dedicated `= 1` form).
- Whether `xo proto gen` is a subcommand or a `go generate` style `xo gen`.
- Proto2 in the first landing or after proto3.
