# A landing placed late reads variables written after the faulting operation

Status: Observed (2026-10-09, decision 0085)

Symptom: clang rejected the LLVM backend's IR of `defer_blocks`:
`Instruction does not dominate all uses!` for the result of a checked
add used by `xo_rs_concat_int` in deferred code. Earlier, the deferred
code of `divide` (`defer ...; a / b`) did not run on the fault at all.

Cause: the builder places the landing of an operation that can fault
inside a block with deferred code when the next instruction is added
(the caller sets the operation's `Pos` after adding it). By then the
statement may have assigned the operation's result to a variable
(`total += x`), and the block may have ended (its deferred frame popped).
The cleanup lowered at that point read `total` as the new value, which
does not exist when the add faults, and saw the frames as they were
later.

Fix: `s.pendingAt` snapshots the open blocks and scopes (with the count
of their deferred statements) when the operation is added, and
`s.pendingDefs` keeps the definitions its block replaced since; the
landing's cleanup is lowered against both (`internal/ssa/landing.go`
flushLanding). Code that adds an instruction and then writes variables
before the next one must keep going through `s.add` and `s.writeVar`.

2026-10-21: the cleanup is now a shared block (deferred-code-copied-per-landing.md)
that reads the variables through the faulting operation's block when it
is sealed, so flush_landing leaves that block with the definitions from
before the operation and moves the later ones to the continuation block.
