# A rune literal was not a number: `Int('a')` and `Rune(n)` did not compile

Status: Observed (2026-10-08, porting the lexer to Xo, `selfhost/lexer`).
Resolved (2026-10-08) by decision 0079.

Symptom (before 0079): byte level code could not compare a byte with a
character. `Int('a')` was `XO0409` ("cannot convert `Rune` to `Int`;
numeric conversions take a number"), `Rune(97)` was `XO0407` ("`Rune` is
a type and cannot be called"), and `b[i] == 'a'` did not type check (`b[i]`
is `UInt8`).

Cause: `Rune` is not a numeric type, so the numeric conversions (core 2.1)
did not apply to it, and there is no byte literal.

Fix: write the rune literal where the integer is expected: `b[i] == '"'`,
`match c { 'a'..='z' => ... }` on a `UInt8`, `const QUOTE: UInt8 = '"'`
(core 1.6). Convert values with `Int(r)`, `Rune(n)`, or `Rune.try(n)`
(core 2.1).

Remaining trap: a non ASCII rune literal where a byte is expected is
`XO0401` (`'é'` is two bytes in UTF-8, never one byte); compare with its
bytes or decode runes (`Bytes.decode_rune`, std.md 2.2) instead.
