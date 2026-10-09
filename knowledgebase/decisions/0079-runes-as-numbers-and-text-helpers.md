# 0079: Runes convert to and from integers; Unicode and quoting helpers

Status: Accepted (2026-10-08). Implemented in `std/prelude.xo`, `internal/check` (`expr.go`
rune literals, `call.go` conversions), `internal/gen`, `std/rt`
(`RuneOf`, `Rune_try`, the text helpers in `str.go`). Gaps 2 and 4 of
`bench/results/selfhost-lexer-2026-10-08.md`.

## Context
Byte level code (the self hosted lexer) compares bytes with characters all
the time. A `Rune` was not a number: `Int('a')` was `XO0409`, `Rune(97)`
was `XO0407`, there was no byte literal, and `b[i] == 'a'` did not type
check (`b[i]` is `UInt8`). The lexer named 50 byte constants
(`const C_QUOTE = 0x22`) and matched its operators on hex numbers.

## Decision
- **Rune literals take an integer type from context.** Where an integer
  type is expected (an annotation, a parameter, the other operand of an
  operator, a pattern on an integer, an integer `const`), a rune literal is
  that type's value of its code point, as an integer literal takes the
  expected integer type (core 1.6). Elsewhere it is a `Rune`, as before.
- **8 bit types take only ASCII.** `'é'` is U+00E9, which fits a `UInt8`,
  but it is two bytes in UTF-8, so `b[i] == 'é'` would silently never
  match a UTF-8 source. A rune literal above U+007F where `Int8` or `UInt8`
  is expected is `XO0401` with a note; `Int16`/`UInt16` reject code points
  that do not fit. Wider types take every rune.
- **Conversions both ways.** `Int(r)` and every sized integer type of a
  `Rune` (checked like any integer conversion: `UInt8('é')` faults);
  `Rune(n)` of any integer type, faulting when `n` is not a Unicode
  scalar value; `Rune.try(n: Int) -> Rune?`, the shape of `Int32.try`.
  `Float(r)` stays `XO0409`, now with a note pointing at `Int(r)`; an
  operator between a `Rune` and an integer (`XO0406`) gets a note naming
  the conversion and the literal rule.

- **Unicode and quoting in the prelude, with Go's exact rules** (std.md
  2.2): `Rune.is_print` (`strconv.IsPrint`), `Rune.quote`
  (`strconv.QuoteRune`), `Str.quote` and `Bytes.quote` (`strconv.Quote`),
  `Bytes.rune_count` and `Bytes.decode_rune(i) -> (Rune, Int)`
  (`unicode/utf8`), next to the existing `is_letter`, `is_digit`,
  `is_space` (`unicode`). Matching Go byte for byte is the point: the
  self hosted toolchain must print the messages the Go one prints, and an
  agent porting Go code expects `%q`. The Unicode tables are those of the
  Go release that builds the compiler; the native runtime carries tables
  generated from it and a test compares every rune (decision 0081).

## Consequences
- `match c { 'a'..='z' | '_' => ... }` and `c == '"'` work on `UInt8`
  bytes with no named constants. Duplicate arm detection treats `'a'` and
  `97` on an integer as the same arm.
- A `Rune` value never mixes with an integer silently: only literals adapt,
  as integer literals do.
- The native backends lower an integer typed rune literal as an integer
  constant (decision 0081).

## Alternatives
- **Byte literals (`b'a'`)**: one more literal form and token kind for
  what the expected type already says; the rule above is how Go's untyped
  rune constants behave.
- **`Rune` as a numeric type**: would allow `r + 1` and `r * r`, which are
  meaningless on characters, and would make `Rune` interpolation ambiguous.
- **`Rune.from_int(n) -> Rune?` only**: the spec already spells checked
  conversions `T(x)` and partial ones `T.try(x)`; a third shape for one
  type would be one more thing to remember.
