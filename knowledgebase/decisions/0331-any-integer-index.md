# 0331: any integer type indexes a List or Bytes

Status: Accepted (owner, 2026-10-21). Implementation: Xo Publish; the
spec text in core 3.6 lands with the checker change.

## Context

`xs[i]` on `List` and `Bytes` takes only `Int` today; any other integer
type is error `XO0401` ("expected `Int`, found `UInt8`"). Byte parsers
hold positions and lengths in the widths they read from the wire
(`UInt8`, `UInt16`, `UInt32`), so ports of Go's parsers fill with
`b[Int(i)]`. The 2026-10-21 language poll named the casts among the most
common friction in std and compiler code.

## Decision

The index of `xs[i]` (read, and assignment on a `var` List or `Buf`) may
be any integer type: `Int`, `Int8`..`Int64`, `UInt8`..`UInt64`. The
value is widened to `Int` (a `UInt64` above `Int`'s range is out of
range) and bounds-checked as today: out of range faults, `get(i)` takes
the same types and returns `T?`. Slice bounds (`xs[a..b]`) follow the
same rule. No other implicit numeric conversion is added (core 2.1 is
unchanged); `Map` keys keep their declared type.

## Consequences

- Indexing code drops its casts; no existing program changes meaning.
- Checker change only: each backend already lowers an index from an
  `Int`, so the widening is a sign or zero extension before the check.
- Spec core 3.6 states the rule when the checker lands.

## Alternatives

- **Keep `Int` only:** explicit, but the casts carry no information (the
  bounds check catches every bad value).
- **Implicit widening everywhere:** rejected, core 2.1 keeps conversions
  explicit outside indexing.
