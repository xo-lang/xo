# Native backends order NaN differently from the Go backend

Status: Observed (2026-10-08, found writing `internal/arm64/testdata/run/select`)

Symptom: a program comparing Float struct fields with `<` where some
values are NaN (`0.0 / 0.0`) prints different results under the Go
backend than under `--backend=llvm` and `--backend=arm64` (which agree
with each other).

Cause: `spec/std.md` makes `Float` `Ord` with a total order in which NaN
sorts after every other value; the Go backend implements `<` through
`rt.FloatCmp` (`std/rt/arith.go`). The LLVM and arm64 backends lower `<`
to the IEEE comparison (`fcmp olt`, `fcmp` + `b.mi`), which is false
whenever an operand is NaN.

Fix: not fixed yet. The native backends should lower Float `<`, `<=`,
`>`, `>=` to the total order (NaN last), or the spec should say `<` is
IEEE and only `Ord` based sorting is total; that choice needs a
decision. Until then, `testdata/run` programs that compare against the
Go backend avoid NaN in ordered comparisons.
