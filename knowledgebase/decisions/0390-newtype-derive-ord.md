# 0390: `derive Ord` on a newtype

Status: Accepted (lead, 2026-10-27, under the owner's go-ahead on decision
0387 to make `uuid.Uuid` comparable). Proposed by Xo Lang 2.

## Context

Decision 0387 (accepted by the owner, option (c)) makes `uuid.Uuid`
comparable with `u < v` in 0.33.0. A `Uuid` is a newtype,
`type Uuid struct(Str)`, and newtypes could not be ordered: `Ord` came
only from `derive Ord` on a struct or an enum (std.md 1, the checker's
`is_ord`), and the parser took no `derive` after a newtype
(`type U struct(Str) derive Ord` was XO0175; `a < b` on it XO0404).
Decision 0097 gave newtypes their value's equality, hashing, and JSON
automatically, but not its order.

## Decision

- `derive Ord` may follow a newtype declaration (core 2.8, the grammar's
  TypeDecl). The newtype then orders as its value: `<`, `<=`, `>`, `>=`,
  `sort`, `min`, `max`, `clamp`, and `T: Ord` code.
- The value's type must be `Ord`; otherwise `XO0404` at the derive
  ("`derive Ord` needs the newtype's value to implement `Ord`"). A
  generic newtype (`type Wrap[T] struct(T) derive Ord`) whose value is a
  type parameter is not checked at the derive, as a generic struct's
  fields are not.
- `Ord` is the only derive a newtype takes: any other is `XO0404` ("a
  newtype derives only `Ord`"), since its equality, hashing, and JSON are
  its value's already; derive arguments are a parse error.
- No new diagnostic code. The parse's binary form changes (TypeDecl
  carries its derive list): AST_CODEC_VERSION is v2.
- Backends: a newtype is lowered as a one field struct on every backend
  (decision 0097), so the native backends' derived struct order
  (`xo.cmp[T]`, field by field) and the Go backend's `XoCmp` cover it
  once the checker records the derive. `min`, `max`, and `clamp` on the
  native backends took only numbers and Str (XO1201 on a derived Ord
  struct too); they now call the type's `xo.cmp[T]`. Golden `newtype_ord`
  (arm64, LLVM, wasm, Go parity); errorcheck `newtype_derive`.

## Consequences

- `uuid.Uuid` can `derive Ord` (decision 0387), and so can any wrapper
  (`UserId`, `Email`, units) that should sort as its value.
- Ordering stays opt in, as for structs: an existing newtype does not
  become ordered.

## Alternatives

- **Every newtype over an Ord type is Ord automatically**, extending
  0097's automatic equality and hashing. No syntax, but `Email < Email`
  becomes legal everywhere, which a type author may not want, and it
  differs from structs, whose order is opt in. Rejected (lead).
- **A hand written order** (`fn (a: Uuid) cmp(b: Uuid) -> Int`). Xo has no
  user written `Ord` methods (std.md 1: Ord is a builtin interface), so
  this would be a larger change. Rejected.
- **Give `uuid.Uuid` an order in the compiler only.** A std special
  case; any user newtype would still need the same. Rejected.
