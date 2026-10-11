# A new xo.mod example in core.md fails TestSpecCoreExamples

Status: Observed (2026-10-09, adding the `deny` example to core 4.2)

**Symptom.** `go test ./internal/parser` fails with
`core.md:NNN: example does not parse ... XO0168: statements are only
allowed inside a function or test body, found identifier `module``.

**Cause.** `internal/parser/spec_test.go` parses every fenced block of
`spec/core.md` as Xo unless the block's first line starts with a prefix
in `specSkips`. xo.mod blocks were skipped only for
`module github.com/acme`; a block for `module example.com/notes` was
parsed as code.

**Fix.** Add a `specSkips` entry for the new first line (there is now one
for `module example.com/`), or start the example with an existing prefix.
Same for xo.work blocks (`xo preview`) and shell blocks (`xo query`,
`xo edit`).
