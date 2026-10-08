# `#{...}` is a Set literal, not a Map literal

Status: Observed (2026-10-08, `internal/fix` tests)

Symptom: `let m: Map[Str, Int] = #{"a": 1}` fails with XO0401 (expected
`Map[Str, Int]`, found `Set[Str]`), XO0160 at the `:`, and XO0168 on the
following lines because the parser lost its place.

Cause: `#{` starts a Set literal. Map literals are plain braces in
expression position (`{"a": 1}`, `{}` for an empty map, core 15).

Fix: write `{"a": 1}` for a map. The cascade of XO0168 errors after a
broken literal is a sign to look at the first error only.
