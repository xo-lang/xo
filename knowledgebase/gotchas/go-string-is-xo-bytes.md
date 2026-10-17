# A Go `string` ports to Xo `Bytes`, not `Str`

Status: Observed (2026-10-08, porting the lexer to Xo, `compiler/lexer`)

Symptom: Go code that slices strings at byte offsets and keeps arbitrary
bytes (comments, string literal bodies, an invalid character in a message)
cannot use `Str`: `Bytes.to_str()` is `None` for bytes that are not UTF-8,
and slicing a `Str` inside a UTF-8 sequence faults (core 3.6). A port on
`Str` would lex a file with a stray `\xff` differently from the Go lexer,
or fault.

Cause: Xo `Str` is UTF-8 text by definition (core 2.1); Go strings are
byte strings.

Fix: hold source and every derived text as `Bytes` (`os.fs.read`, `b[i]`,
`b[a..b]`, `+`), and convert to `Str` only for text known to be ASCII
(identifiers, with `to_str().or_fault(...)`) or output already escaped to
ASCII (the dump quotes every byte outside printable ASCII as `\xHH`). Build
`Bytes` with `BytesBuilder` (decision 0078, std.md 2.7), not with `out =
out + more`, which copies `out` each time. Bytes still lacks the `Str`
search methods (gap 6 of `bench/results/selfhost-lexer-2026-10-08.md`).
