# Native backends do not check `ensures` clauses

Status: Resolved (2026-10-09, decision 0085; observed 2026-10-09, golden
`contracts`, found by TestGoldenHybrid)

Symptom: `xo run --backend=arm64 internal/gen/testdata/golden/contracts`
printed `3` and exited 0 where the Go backend faults with `ensures failed:
result > n at main.xo:31:11` (want.txt). The first hybrid build of the
golden compiled `buggy` to assembly and printed the same wrong result.
Struct invariants were not checked either (construction, `with`, `var`
methods).

Cause: the SSA builder lowered `requires` clauses but skipped `ensures`
and invariants; nothing reported it (the XO1201 check passed the
program).

Fix: decision 0085. The builder wraps a function with `ensures` (or a
`var` method of a type with an invariant) around its body and checks
the clauses on success, and checks invariants after literals and
`with` (`internal/ssa/build_contract.go`); goldens
`internal/llvm/testdata/golden/contract_*` compare all three backends.
Hybrid builds no longer exclude these functions. When a new contract
form is added to core 9.1, lower it in build_contract.go and add it to
those goldens.
