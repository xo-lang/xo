# `??` on a nested optional produced Go that did not compile

Status: Observed (2026-10-10, selfhost/check `fs.stat(p).ok() ?? None`);
fixed.

Symptom: `xo build selfhost/check` (Go backend) failed with "cannot use
x (variable of struct type rt.Option[rt.Option[rt.FileInfo]]) as
rt.Option[rt.FileInfo] value"; the checker accepted the program and LLVM
built it. `fs.stat` returns `FileInfo? ! FsErr`, so `.ok()` is a
`FileInfo??` and `?? None` unwraps one level to a `FileInfo?`.

Cause: internal/gen `coalesce` kept the whole left optional whenever
the result type was an optional, which is right for `a ?? b` with both
`T?` but not for `T?? ?? None`.

Fix: keep the left value only when its Go type is the result's; else
take `.V`. Golden: `internal/gen/testdata/golden/nested_option_coalesce`
(also run natively). `T??` cannot be written as a type (the lexer reads
`??`); write `Option[T?]`.
