# arm64: the reference counting helper of a value over about 500 words fails

Status: Observed (2026-10-10, selfhost/parser on `--backend=arm64`)

Symptom: after adding an optional `bound: Expr?` to the `ForStmt` and
`WhileStmt` variants of selfhost/parser's `Stmt` enum, `TestSelfhostParserDifferential/arm64`
failed with `arm64 backend: reference counting of MatchArm (too many
words)`. The Go and LLVM backends built the same program.

Cause: Xo values are held by value, so a `MatchArm` holds a whole
`Stmt`, which holds `Expr` values; one more `Expr` in the largest `Stmt`
variant took `MatchArm` past the 4096 byte frame of its retain and release
helpers (`internal/arm64/rc.go`: the frame is `sub sp, sp, #imm12`, and
the helper copies every word of the value into it). Decision 0128 lifted
the limit for function frames, not for these helpers.

Fix used: the field is `bound: List[Expr]` (zero or one element; a List is
one word), dumped as `bound.first()`. Next step for the backend: allocate
helper frames past 4 KiB as function frames do (`probeSP`, `sub sp, sp,
#hi, lsl #12`), with a test value of 600 words in
`internal/arm64/testdata/run/wide_values`.
