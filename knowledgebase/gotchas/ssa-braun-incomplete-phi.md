# SSA builder: an incomplete block parameter looked up by variable

Status: Observed (2026-10-08, `internal/ssa`, LLVM golden `loops`)

Symptom: `ssa.Build` panicked with `index out of range [-1]` in
`addArgs` while sealing a loop header, for a program with a `while true`
loop whose body assigned a variable it also read.

Cause: in Braun et al.'s construction a read of a variable in an
unsealed block adds a block parameter and records the block as
incomplete for that variable. On sealing, the builder looked the
parameter up again as `defs[var][block]`. A later write to the same
variable in that block replaces that entry, so sealing got the new
value, which is not a parameter of the block (index -1).

Fix: record the incomplete parameter itself (`fn.incomplete` holds
`*Value`s) and recover the variable from `paramVar` when sealing. This
is the paper's `incompletePhis[block][variable] = phi`, which keeps the
phi, not the variable.
