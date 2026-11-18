# x86: a K_PLAIN instruction's clobbers are not saved by the prologue

Status: Anticipated (2026-11-18, the x86 Keccak kernel, task #271).

A K_PLAIN Inst's `clobbers` tell the register allocator which registers
the text overwrites, so no live value stays in them across it. They do
not reach the frame's callee-saved list: the prologue saves only the
callee-saved registers the allocator itself assigned. A kernel that
names rbx, r12, r13, r14 or r15 in its clobbers and overwrites them
returns to its caller with them changed, a miscompile in a caller far
away (any function that kept a value in rbx across the call).

The same kernel cannot keep scratch below rsp either: a leaf function's
spills already sit in the red zone (stores at -0x18(%rsp) and below), so
the text's own frame starts past it.

Fix: a kernel that needs callee-saved registers saves and restores them
itself in its own frame, and that frame is made with
`leaq -(128+N)(%rsp), %rsp`, skipping the red zone (compiler/ssa
x86keccak.xo: rbx and r12 to r15 at 408 to 440 of a 704 byte frame).
Only the caller-saved registers it overwrites go in `clobbers`
(keccak_x86_gprs). The LLVM path needs neither: its inline asm names
every register in its constraint list and LLVM saves what it must.
