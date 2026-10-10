# The RC verifier dropped a function's parameters at an unwinding check

Status: Observed (2026-10-10), fixed.

Symptom: `xo build --hybrid` of any program that starts a task (`scope`,
`os.tasks.spawn_background`) printed `hybrid: no assembly (ssa:
xo.run[Task[...]]: b1: v10 uses v1, which is not alive ... v1 is held -1
times at ret)` and built plain Go: no function ran in assembly.

Cause: hybrid builds the SSA without fault unwinding (`NoFaultUnwind`).
The task runner `xo.run[...]` still branches on `rt.unwinding` (a
cancelled task), but with no call carrying a landing pad there. The
verifier treats the cleanup edge of such a check as "the operation
checked produced nothing" and, when it finds no site, deletes the block's
parameters on the assumption that they are an inlined call's results
(internal/arm64 inline.go). The block was the entry block, whose
parameters are the function's: the Task parameter vanished from the
held set on that edge.

Fix: `internal/ssa/verify_rc.go` only drops parameters of a block that is
not the function's entry block. The program was right all along; only
the verifier was wrong. Check with `m.Verify()` after `InsertRC` on a
module built with `ssa.Options{NoFaultUnwind: true}`.
