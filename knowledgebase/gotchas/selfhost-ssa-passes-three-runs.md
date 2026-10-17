# Three runs take most of TestSelfhostSSA's pass stages

Status: Observed (2026-10-11, again 2026-10-12)

Symptom: after SSA stage 8, TestSelfhostSSA with only the go backend took
about 230 s (the toolchain notes said 25 s before the pass stages), so a
go only "merge" tier still missed its 5 minute target.

Cause: timing the go backend's Xo builder on each of the 646 repository
runs (`--stages` vs build only) showed 157 s of passes, 112 s of it in
five runs and 110 s in three: compiler/ssa (69 s), compiler/check (22 s),
and internal/arm64/testdata/run/huge_frame (19 s; its build stage alone
is 4 s). huge_frame is one 5,000 line function, so the cost is not
proportional to source size: compiler/parser (7,900 lines) takes 1.4 s.
The Go builder's passes (the parent test's 86 s) are skewed the same way.

Fix: the merge tier compares only the build stage of `buildOnlyMerge`
(ssa_test.go at the time; now mergeHeavy in tier_test.go) and every stage of the other 643 runs; TestSelfhostSSA
fell from 231 s to 54 s. The full tier still compares every stage of
every run. When a new large program (a self hosted component, a huge
function fixture) slows the merge tier, time it the same way (build
compiler/ssa with `xo build -o`, run it per directory with and without
`--stages`) and add it to the list.

Recurrence (2026-10-12): the fixed lists (buildOnlyMerge in ssa_test.go,
arm64Heavy in arm64_test.go) missed the components added since,
compiler/arm64, compiler/llvm, and compiler/xo, so the merge tier ran
the SSA passes of all three, the arm64 emitter on compiler/llvm, and the
LLVM emitter on all three: 1,329 s under load instead of about 2.5 min.
Fix: one rule, `mergeHeavy` in tier_test.go (huge_frame and every
directory under compiler/ but lexer and parser), used by the SSA,
arm64, and LLVM comparisons, so a new component is heavy without an
edit. Figures: knowledgebase/infrastructure/toolchain.md, the merge
tier line.
