# Native backends do not check `ensures` clauses

Status: Observed (2026-10-09, golden `contracts`, found by TestGoldenHybrid)

Symptom: `xo run --backend=arm64 internal/gen/testdata/golden/contracts`
prints `3` and exits 0 where the Go backend faults with `ensures failed:
result > n at main.xo:31:11` (want.txt). The first hybrid build of the
golden compiled `buggy` to assembly and printed the same wrong result.

Cause: the SSA builder lowers `requires` clauses (`fn.requires` in
`internal/ssa/build.go`) but skips `ensures`; nothing reports it (the
XO1201 check passes the program).

Fix: hybrid builds exclude functions with an `ensures` clause
(`GoOptions.Exclude`, set in `internal/build/hybrid.go`) and functions
that hold a struct with an invariant (the Go backend checks it at
construction, SSA does not). For the native backends, the next step is
to lower `ensures` at each return in the SSA builder (bind `result` and
the `old` values, fault with the Go backend's message) or report it as
XO1201 until then.
