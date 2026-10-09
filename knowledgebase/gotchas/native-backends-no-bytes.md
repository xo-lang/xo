# The native backends do not build programs that use `Bytes`

Status: Observed (2026-10-08, `xo build --backend=arm64 selfhost/lexer`)

Symptom: `xo build --backend=arm64` (and `--backend=llvm`) on the self
hosted lexer fails with 274 errors such as "the SSA builder does not
support Bytes", "... the type `UInt8`", "... the method `Fs.read`", and 91
times "the SSA builder does not support this expression".

Cause: the shared SSA builder (`internal/ssa`, decision 0045) lowers a
subset of the language: no `Bytes`, `UInt8`, `UInt64`, `StrBuilder`,
ranges outside `for`, `main` with `!`, or the `Fs`, `Env`, and `Proc`
methods a CLI needs.

Fix: none yet; build such programs with the Go backend. Next experiment
(gap 3 of `bench/results/selfhost-lexer-2026-10-08.md`): lower `Bytes` like
`Str` with a byte index op and `UInt8`/`UInt64` as unsigned integers in
`internal/ssa`, then rerun the build and work down the remaining list.
