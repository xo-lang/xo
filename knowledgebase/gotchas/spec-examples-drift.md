# Spec examples contradict the spec

Status: Observed (2026-10-07, paper test task 01; parser check added 2026-10-07)

Appendix A wrote `"GET /users/{id:Int}"`, which section 1.6 interpolated at
the time. Agents copy examples literally, so a wrong example spreads into every
solution.

Now checked automatically: `internal/parser/spec_test.go`
(`TestSpecCoreExamples`) extracts every fenced block in `spec/core.md` and
parses it as a file, then as a function body, then in signature mode. When you
change the spec or the grammar, run `go test ./internal/parser/`:

- A new non Xo block (output sample, shell commands) needs an entry in
  `specSkips`.
- An example that cannot parse because the spec itself is wrong goes in
  `specKnownFailures` with the reason, and the spec should be fixed. The list
  is empty since spec 0.4 (2026-10-07): local `const` made the core 3.1 block
  valid.
- `{ ... }` in an example is treated as an elided body and replaced by `{}`.
- The test fails when a known failure starts parsing, so the list stays honest.

Type checked too (2026-10-07): `internal/check/corpus_test.go`
(`TestSpecAppendixA`) checks Appendix A as a module. Known diagnostic:
`use std/json` is never used (XO0110), because `derive Json` does not
reference the module. Either the example drops the import or derives count
as a use; until the spec decides, the test lists it as known.

Update (spec 0.4, 2026-10-07): the example dropped `use std/json`, so
Appendix A checks clean and `specAppendixKnown` is empty. std.md 7 writes a
result type on its own line (`-> List[Result[R, E]] uses fx`), which core 1.3
does not list as a continuation; the parser accepts it (reading 7 in
`bench/results/check-corpus-v0.4-spec.md`).
