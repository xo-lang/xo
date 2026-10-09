# A rune literal is not a number: `Int('a')` and `Rune(n)` do not compile

Status: Observed (2026-10-08, porting the lexer to Xo, `selfhost/lexer`)

Symptom: byte level code cannot compare a byte with a character. `Int('a')`
is `XO0409` ("cannot convert `Rune` to `Int`; numeric conversions take a
number"), `Rune(97)` is `XO0407` ("`Rune` is a type and cannot be called"),
and `b[i] == 'a'` does not type check (`b[i]` is `UInt8`).

Cause: `Rune` is not a numeric type, so the numeric conversions (core 2.1)
do not apply to it, and there is no byte literal.

Fix (workaround until the gap is closed): compare `Int(b[i])` with named
constants (`const C_QUOTE = 0x22`, as in `selfhost/lexer/lexer.xo`) and
match on hex literals. To make a `Bytes` from a code point, encode it
yourself and go through `Bytes.from_hex` (`encode_rune` in
`selfhost/lexer/text.xo`). Proposed fix: gap 2 of
`bench/results/selfhost-lexer-2026-10-08.md`.
