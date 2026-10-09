# The native backends did not build programs that use `Bytes`

Status: Observed (2026-10-08, `xo build --backend=arm64 selfhost/lexer`).
Resolved (2026-10-08) by decision 0081.

Symptom (before 0081): `xo build --backend=arm64` (and `--backend=llvm`)
on the self hosted lexer failed with 274 errors such as "the SSA builder
does not support Bytes", "... the type `UInt8`", "... the method
`Fs.read`", and 91 times "the SSA builder does not support this
expression".

Cause: the shared SSA builder (`internal/ssa`, decision 0045) lowered a
subset of the language: no `Bytes` indexing or slicing, `UInt8`,
`UInt64`, `StrBuilder`, ranges outside `for`, or the `Fs`, `Env`, and
`Proc` methods a CLI needs.

Fix: decision 0081 lowers them; `selfhost/lexer` builds and matches the Go
lexer on both native backends. Still outside the subset (2026-10-08):
the other `Fs` methods (`write`, `read_dir`, `walk`, ...), `Env.vars`,
`Proc.run`, `Float32`, list patterns, and `xo test`. A construct outside
it is a build error that now names its AST kind.
