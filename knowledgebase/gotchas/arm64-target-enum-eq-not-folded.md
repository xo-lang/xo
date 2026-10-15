# arm64: `target.os == TargetOs.X` is an equality call at run time

Status: Observed (2026-10-15, the zero-c perf sweep,
bench/results/zero-c-perf-sweep-2026-10-15.md); fixed the same day in
selfhost/ssa (below)

Symptom: a macOS fast path added to `sysio.clock_now`
(std/native/llrt/sysio.xo) as `if target.os == TargetOs.Darwin &&
target.arch != TargetArch.Wasm32 { ... }` made sysio's `clock`
workload 8% slower on arm64 (100.3M to 108.4M instructions), while
LLVM got 4% faster.

Cause: the arm64 backend does not fold `==` or `!=` on the target
enums. The clock_now it emitted called two enum equality helpers
(`_xo_h...`) on every Clock.now. A `match` on the same constants
compiles to a compare and a branch. LLVM folds both forms.

Fix: write target tests as `match target.arch { ... }` and
`match target.os { ... }`, as `sysio.layer()` does. With the match, the
same fast path took clock to 92.8M on arm64 (main: 93.3M). Four `==`
tests remain in std/native/llrt (linux.xo, stdin.xo), on paths that
are not hot. Folding them in selfhost/arm64 (constant enum operands)
would remove the trap.

Fixed (2026-10-15): selfhost/ssa/expr.xo compare_values folds `==` and
`!=` on two variants known at build time to a constant, for every
backend. Those are payload-free variants, made directly or read from a
field of a struct of them (the predeclared `target`). test/codegen/enums
checks that no call or compare is left on arm64 and LLVM. clock_now
went back to `==`, and that took sysio clock to 86.5M instructions on
arm64 (0.93 of main), below the match version, since the branch is gone.
