# `t.0.1` lexes as `t`, `.`, and the float `0.1`

Status: Anticipated (2026-10-07, adding tuple fields `t.0`, core 3.5)

Symptom: a nested tuple field `pair.0.1` would be the selector `pair.`
followed by the float literal `0.1`, a parse error or a wrong tree.

Cause: the number scanner takes `digits . digits` as a float wherever it
starts.

Fix: in `internal/lexer`, a number right after a `.` token is a plain
decimal index: no fraction, no exponent. `0.5` elsewhere is still a float,
and ranges (`0..n`) are unaffected because `..` is its own token. The parser
rejects an index written with a prefix or `_` (`t.0x1`). Rust has the same
trap.
