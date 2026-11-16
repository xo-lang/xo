# 0683: derive Proto gains decode options and a message descriptor (task #102)

Status: Accepted (owner, 2026-11-16; proposed by Xo Lang 3, 2026-11-16; the lead routed the compiler
part to Xo Lang 1, ADR first; Xo Lang 1 reviews it)

## Context

Task #102 closes std/protobuf's gaps against protobuf-go v1.36 (map.txt).
The std-only rows landed on 2026-11-16 (protojson's EmitUnpopulated,
AllowPartial, RecursionLimit, Multiline and Indent; `xo proto gen` across
files, 95df33e0e). The rows left need the code `derive Proto` generates
(compiler/check/derive.xo, decision 0530), which today is
`proto_marshal__T`, `proto_unmarshal__T`, `proto_size__T`, `proto_write__T`,
`proto_merge__T` and helpers, and nothing that describes the message:

1. **proto.UnmarshalOptions**: `protobuf.unmarshal[T]` takes no options.
   protobuf-go has DiscardUnknown (drop unknown fields instead of keeping
   them in the `proto "unknown"` field), RecursionLimit (default
   protowire.DefaultRecursionLimit, 10000; a deeper input is an error) and
   AllowPartial (proto2 required fields; derive Proto has none, so it is
   always met). The generated decoder (`proto_merge__T`) calls itself for
   each nested message with no depth count, so the only bound on a hostile
   input today is the stack. Measured 2026-11-16 (xo 0.52.0, darwin/arm64,
   a `Node` with a `child: Node?` field): 10,001 levels decode (protobuf-go
   returns its recursion error), 2,000,000 levels (9.5 MB) decode, and
   8,000,000 levels (about 38 MB of input) end the process with "fatal
   error: stack overflow" on arm64 and LLVM. A server that unmarshals
   untrusted input can be stopped by one message.
2. **proto.MarshalOptions**: Deterministic is already the only mode;
   AllowPartial is always met; UseCachedSize has no cache to use. Nothing
   to generate.
3. **A descriptor**: proto.MessageName, fieldmaskpb.New, IsValid and
   Append (a path checked against the message's field names), protojson's
   Any by type URL, and prototext all need the message's full name and its
   fields' names, numbers and kinds, which Xo cannot reflect on.
4. **Extensions** (proto.GetExtension and the rest): proto2 only for
   messages (proto3 allows them only on the descriptor options), and
   derive Proto is proto3 (decision 0530 item 2).
5. **`xo proto gen` across packages**, found 2026-11-16
   (gotchas/derive-proto-across-files.md): an Int enum from another file of
   the package is not taken (XO0601 `proto_zero__K`); a struct of another
   package that derives Proto is refused (XO0425), so a `.proto` importing
   `google/protobuf/timestamp.proto` cannot use std/protowkt's types; the
   derive helpers of a non-pub struct are not visible from another file of
   the same package.

## Decision

1. **Decode options.** `protobuf.unmarshal_with[T: Proto](data: Bytes,
   opts: UnmarshalOptions) -> T ! ProtoErr` with
   `struct UnmarshalOptions { discard_unknown: Bool = false,
   recursion_limit: Int = 10000, allow_partial: Bool = false }`;
   `unmarshal[T]` is `unmarshal_with[T]` with the defaults, so it gains
   the depth check too (protobuf-go's default behavior). The generated
   `proto_unmarshal__T` and `proto_merge__T` take the options and a depth:
   each nested message (and group) decodes at depth + 1, and past
   `recursion_limit` the result is `ProtoErr.Recursion`, as protobuf-go's
   "exceeded maximum recursion depth". With `discard_unknown` the unknown
   field bytes are skipped instead of appended to the `proto "unknown"`
   field. `allow_partial` is accepted and has no effect until proto2
   required fields exist. `merge_from` keeps its signature.
2. **A message descriptor.** derive Proto also generates
   `proto_desc__T() -> protobuf.MessageDesc`:
   ```
   struct MessageDesc { full_name: Str, fields: List[FieldDesc] }
   struct FieldDesc {
     name: Str              // the .proto name (proto_name), e.g. "user_id"
     json_name: Str         // protojson's lowerCamelCase name
     number: Int
     kind: FieldKind        // Go's protoreflect.Kind cases
     cardinality: Cardinality  // Optional, Repeated, Map (Required later)
     oneof: Str?            // the oneof's name for a member
     message: (fn() -> MessageDesc)?  // a message or map value field's type
   }
   ```
   The full name is `derive Proto(name: "pkg.Msg")`, which `xo proto gen`
   writes from the `.proto` package; without it, the Xo module path and
   type name. `message` is a function so recursive messages need no cycle
   in data. std uses it for `protobuf.message_name[T]()`,
   `protowkt.new_field_mask[T](paths) -> FieldMask ! WktErr`,
   `protowkt.is_valid_mask[T](fm) -> Bool` and `FieldMask.append[T]`
   (fieldmaskpb's New, IsValid, Append: each path's segments walk the
   message fields, a repeated or map field ends a path), protojson's Any
   for types the program names, and a later std/prototext (writer and
   reader over the descriptor and the wire, protobuf-go's prototext as the
   oracle), which needs no other generated code.
3. **Extensions are not planned** while derive Proto is proto3 only; the
   map.txt rows say so and point here. They come with proto2 support
   (required, defaults, groups as fields, extensions), which is its own
   ADR.
4. **`xo proto gen` across packages**: the three compiler fixes in
   Context item 5 (a cross-file Int enum's `proto_zero__K`, derive Proto
   helpers of a struct in another package found through its module, and
   the helpers of a non-pub struct visible to the package's other files),
   so the generator maps `google/protobuf/*.proto` imports onto
   std/protowkt's types instead of generating copies. Each fix lands with
   the failing case from gotchas/derive-proto-across-files.md as a test.
5. **Order and owners.** Xo Lang 1: item 1 first (it closes the stack
   overflow in Context item 1, a bug fix that lands with the 8,000,000
   level input as its failing test), then item 4's fixes, then item 2's
   generated function. Xo Lang 3: the std side of each (the
   options struct and `unmarshal_with`, the descriptor types, fieldmask's
   functions, message_name, protojson's Any by name, then prototext), each
   checked against protobuf-go in the protobuf corpus (scripts/stdcorpus,
   its protoref oracle) with a timing row for `unmarshal_with`.

## Consequences

- A hostile input nested past 10000 levels is an error, not a stack
  overflow, on every decode path (a test with a self-referencing message
  at depth 10001 and at 10000, as protobuf-go's TestRecursionLimit).
- Each derive Proto type grows by one small function; programs that never
  call the descriptor's users drop it (dead code elimination as for the
  other derive helpers).
- `protobuf.MessageDesc` becomes public API; it is a subset of
  protoreflect's descriptor, enough for names, masks and text formats, not
  dynamic messages (decision 0530 keeps `protobuf.Dynamic` for later).

## Alternatives

- **Options as globals or environment**: rejected; std keeps no
  process-wide mutable state, and protobuf-go passes them per call.
- **A depth limit in std only** (counting in `protobuf.unmarshal` around
  the generated call): cannot see nesting inside the generated decoder.
- **Descriptors from the serialized FileDescriptorProto** that `xo proto
  gen` could embed (decision 0546 item 2): complete, but needs a
  descriptor parser at run time and covers only generated types, not hand
  written derive Proto structs. The generated `proto_desc__T` covers both;
  embedding stays possible for gRPC reflection.
- **Extensions now**: they need proto2's presence rules first; doing them
  alone would add a registry with no message able to use it.

## Addendum 2026-11-16: item 1 landed (Xo Lang 1, task #127)

The compiler and std halves of item 1 landed together: `UnmarshalOptions`
and `unmarshal_with` in std/protobuf.xo, and derive Proto's
`proto_unmarshal_with__T`, with `proto_mergedepth__T` taking the depth
left and a DiscardUnknown flag that nested messages inherit. The
recursion limit itself and its 8,000,000 level test landed earlier with
#132 (3284ae078). Golden proto_options. Items 4 and 2 are next.

## Addendum 2026-11-16 (2): item 4 landed (Xo Lang 1, task #127)

derive Proto takes an Int enum of any file of the package and a type of an
imported module (`protowkt.Timestamp`, an imported package's enum as
`m.K`): load_dir expands derive after the module's files parse and its
imports load. Golden proto_cross_file. std/protoparse's gen_set still
refuses these types; mapping `google/protobuf/*.proto` onto std/protowkt
is std's (Xo Lang 3). Item 2 (the descriptor) is next.

## Addendum 2026-11-16 (3): item 2 landed (Xo Lang 1, task #127)

derive Proto generates `proto_desc__T` (and `proto_descriptor__T`,
`proto_message_name__T` behind `protobuf.descriptor[T]()` and
`message_name[T]()`); the types are in std/protobuf.xo as above, with
`FieldKind` and `Cardinality` deriving Eq and Display. Choices the ADR
left open: a oneof member's name is its variant's name in snake case, at
the oneof's position; a map field's `kind` and `message` are its value's;
`derive Proto(name: ...)` is the only argument derive Proto takes
(XO0413). Golden proto_descriptor. The std users (fieldmask, protojson's
Any, prototext) are Xo Lang 3's.

## Addendum 2026-11-16 (4): fieldmask's functions (Xo Lang 3, task #102)

`protowkt.new_field_mask(md, paths)`, `FieldMask.append_paths(md, paths)`
and `FieldMask.is_valid_for(md)` take a `protobuf.MessageDesc` rather than
a type parameter: a std generic cannot hand its `T` to
`protobuf.descriptor[T]()` (XO0427, derive Proto's functions need the
concrete type at the call), so the caller passes
`protobuf.descriptor[T]()`. `append` keeps its std/protodesc form, hence
`append_paths`. The extension rows in map.txt say not planned and point
to item 3.
