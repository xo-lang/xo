# A new MIR block parameter without edge moves compiles and crashes

Status: Observed (2026-10-08, internal/arm64, pointer induction variables)

Symptom: after adding a pointer parameter to loop headers
(`internal/arm64/lsr.go`), sort's native binary built, its assembly read
`ldr x11, [x20, #24]` in the loop, and it died with SIGSEGV (exit 139):
x20 was never written.

Cause: the hook that adds the parameter's moves to every incoming edge
(`return s.lsrMoves(e, dsts, srcs)` at the end of `edgeMoves`) had not
been applied (a scripted edit matched the wrong indentation and changed
nothing). MIR virtual registers are not SSA and nothing checks that a
register is defined before it is used: liveness simply makes it live
into the function, and the allocator gives it a register nobody writes.

Fix: `regalloc` now fails ("internal: f reads virtual register n before
it is written") when any virtual register is live into the entry block,
so the same mistake is a build error. Also check that an edit script
changed the file (assert the old text is present).
