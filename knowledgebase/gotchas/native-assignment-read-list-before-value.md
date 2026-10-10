# The native backends read an assignment's list before its value

Status: Observed (2026-10-10, selfhost/check built with arm64 and LLVM);
fixed by decision 0140.

Symptom: the arm64 and LLVM builds of selfhost/check faulted with
`index 161 out of range for List of length 161` on modules with a
generic declaration; the Go backend build ran fine. Reduced:
`c.objs[o].kids = c.add_kids()`, where `add_kids` pushes onto `c.objs`,
left `c.objs` with its old length natively.

Cause: internal/ssa built the place `c.objs[o]` (reading the list) before
lowering the value, then stored into the stale list and wrote it back.
The spec did not fix the order, and the Go backend computed the value
first.

Fix: the index place reads its list when first used (as a field place
already did), so a plain assignment reads it after the value. Golden:
`internal/gen/testdata/golden/assign_rhs_mutates`. A program that
works on the Go backend and faults or differs natively is worth reducing
to a golden at once: the self hosted components are the largest native
programs so far and keep finding such cases.
