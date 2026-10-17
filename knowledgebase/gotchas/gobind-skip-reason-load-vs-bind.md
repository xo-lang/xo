# XO1102 read differently after a fresh binding and a cached one

Status: Observed (2026-10-12), fixed (decision 0177).

**Symptom.** The message of XO1102 ("Go `format_complex` in `strconv`
is not bound: ...") ended with the Go name, `(strconv.FormatComplex)`,
when the binding came from the cache, and without it when the same
check had just made the binding. The self hosted checker, which only
reads cached bindings, disagreed with the Go checker on the first run
in a fresh directory.

**Cause.** internal/gobind records a skipped name twice: in
`Binding.Skipped` (the reason alone) and as a `// skip: name: reason
(Go name)` line of the declaration file. `Load` parsed the line and
kept the Go name as part of the reason; `Bind` did not.

**Fix.** `Load` drops the trailing ` (Go name)` (TestMapping compares
`Load`'s Skipped with `Bind`'s), and the self hosted checker
(compiler/check goimport.xo) does the same. Keep any text that both
writes and reads in one place when it feeds a diagnostic.
