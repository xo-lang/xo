# A Go function value Xo cannot send back breaks functional options

Status: Observed (Lang 1, 2026-10-29)

**Symptom.** A binder change that mapped every Go function value given to
Xo to an Xo function (`fn(..) uses go`) lost 14 third-party functions in
the coverage survey: zstd's `with_concurrent_blocks`, goldmark's
`with_parser`, x/text's `cases.handle_final_sigma` and their kin were
skipped with "function value ...: unexported type ...".

**Cause.** They return a named function type (`type EOption func(*encoderOptions)
error`) that exists only to be passed back to Go (`zstd.NewWriter(w,
opts...)`). As an opaque handle it round trips; as an Xo function it cannot
go back to Go (funcValueConv has no toGo), and its unexported parameter type
does not map anyway.

**Fix.** gobind mapResult gives only unnamed function types the function
value form; a named function type stays mapType's by value handle (decision
0416, as built for Go function values). Check a binder mapping change with
the coverage survey (the method of bench/results/usego-coverage-2026-10-29b.md)
and read the newly skipped names, not only the totals.
