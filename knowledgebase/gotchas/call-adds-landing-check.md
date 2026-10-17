# Every call adds a landing check while the body is built

Status: Observed (2026-10-13, decision 0207 stage 2, code reading
while writing compiler/ssa contract.xo check_no_alloc)

Symptom: a rule checked on a function's SSA right after its body is
built ("cannot fault": no unwind block) rejects a function that only
calls another function which cannot fault, such as `rc.dec` calling
`rc.dec_shared`.

Cause: the SSA builders add a check after every call of a function
(`rt.landing`, or `rt.unwinding` with unwinding checks; build.xo
check_then), which creates the function's unwind block; the checks of
callees that cannot fault are removed later (b.checks), after every
body is built. Calls of `rt` intrinsics (b.rt) get none.

Fix: write such functions without calls (std/native/llrt/rc.xo inlines
dec_shared's one atomic into dec), or check after the checks are
removed.
