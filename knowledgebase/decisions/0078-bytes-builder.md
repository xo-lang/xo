# 0078: `BytesBuilder`, a move only builder for `Bytes`

Status: Accepted (2026-10-08). Implemented in `std/prelude.xo`,
`std/rt/list.go` (Go backend), `std/native/prelude.xo` (native backends,
decision 0081), `internal/check` (move tracking). Gap 1 of
`bench/results/selfhost-lexer-2026-10-08.md`.

## Context
`Bytes` is the only Xo type that holds arbitrary bytes (a `Str` must be
UTF-8), so byte level code (the self hosted lexer, protocol encoders) keeps
its text as `Bytes`. There was no way to build one piece by piece: `out =
out + more` copies `out` every time (quadratic on long inputs), and there
was no way at all to make a `Bytes` from a computed byte value; the lexer
went through `Bytes.from_hex(hex2(c))`.

## Decision
- A prelude type `BytesBuilder` with the shape of `StrBuilder` (std.md 2.7):
  `BytesBuilder.new()`, `BytesBuilder.with_capacity(n)`, `write(Bytes)`,
  `write_byte(UInt8)`, `write_str(Str)` (its UTF-8 bytes),
  `write_rune(Rune)` (its UTF-8 encoding), `len()`, and `freeze() ->
  Bytes`, which moves the builder.
- It is move only like `Buf` and `StrBuilder` (core 5.3): the same move
  tracking and the same `XO0510` (whose text now names all three types;
  its meaning is unchanged).
- `write_byte` takes `UInt8`, the type of `b[i]` on `Bytes`, so copying
  bytes needs no conversion; with rune literals as integers (decision
  0079) `out.write_byte('\n')` reads naturally.

## Consequences
- Appends are amortized O(1). The Go backend holds a `[]byte`
  (`rt.BytesBuilder`); `freeze` converts it to the `Bytes` string once.
- The native backends implement it in Xo over a `Bytes` field whose
  `+` appends in place when the builder owns the only reference (decision
  0081).

## Alternatives
- **`Buf[UInt8]` plus `Bytes.from_list`**: works with no new type, but a
  `List[UInt8]` is eight bytes per element on the native backends and
  `write(Bytes)` would be a loop.
- **A mutable `Bytes`**: breaks value semantics (core 5.2) for every
  existing `Bytes` value.
- **`write_byte(Int)`**: every caller holding a byte would convert to Int
  and back; a range fault at the call would hide the real mistake.
