# `xo fmt` puts every element of a multi line list on its own line

Status: Observed (2026-10-08, generating `compiler/lexer/isprint.xo`)

Symptom: a generated `const` table written as pairs per line
(`0xa1, 0xac,`) fails `xo fmt --check`; formatting turns its 710 lines into
1420.

Cause: the canonical layout splits a bracketed list that breaks after its
opening bracket into one element per line (core 10.5, `internal/format`).
There is no table layout.

Fix: generators emit one element per line (`IsPrintTable` in
`internal/selfhost`) so the output is already canonical, and their test
compares with the file on disk. Proposed fix: gap 9 of
`bench/results/selfhost-lexer-2026-10-08.md`.
