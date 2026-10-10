# A diagnostic pointer went stale when the list grew

Status: Observed (2026-10-10, stage 7 of the self hosted checker)

Symptom: some notes and fixes were missing from `xo check --json` with
no pattern: the `impl` habit kept its note in one file and lost it in
another, a Rust closure's `write fn(...) { ... }` fix appeared only when
its body parsed cleanly, and internal/fix's py-map-builtin golden had no
fix for `map(f, xs)`. The self hosted parser and checker, which keep the
index of a diagnostic, had them all.

Cause: `diag.List.Errorf` returned `&l.Items[len-1]` with `Items
[]Diagnostic`. A caller that reported more diagnostics (parsing a body,
checking an argument) before attaching a note or fix through the pointer
wrote into the old backing array once an `append` had grown the slice,
so the note or fix was lost. Whether it was lost depended on the slice's
capacity at the time.

Fix: `Items []*Diagnostic` (internal/diag); a pointer from `Errorf` stays
valid. `List.Values()` copies the diagnostics out where a
`[]diag.Diagnostic` is needed (parser.ParseFile, check.Result). The
py-map-builtin golden gained its fix. Hold a pointer or an index, never
`&slice[i]`, across calls that may append.
