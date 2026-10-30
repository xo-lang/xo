# Windows tracebacks' locals: every value read from memory unavailable

Status: Observed (2026-10-30, lifting decision 0268's Windows gate)

**Symptom.** With the frames' registers from the RtlVirtualUnwind walk
(unwind.xo win_walk_regs) and the exe's DWARF read (dwline.xo vars_load's
pe_sects), XO_TRACEBACK=locals on native windows/amd64 printed a value
held in a register (`k = 0`) but `s = <unavailable>` for a stack slot
and `xs = List[Int] <bad address>` for a List whose register was right
(checked against the DWARF: the frame's pc in its range, rbx a heap
address). Linux printed `s = 1` and `xs = [1, 2, 3]` for the same program.

**Cause.** dwline.xo probe, which checks that memory can be read before
the traceback reads it (a pipe write that fails with EFAULT on Linux and
macOS), had no Windows arm and answered false, so every read through
memory failed.

**Fix.** probe on Windows asks VirtualQuery for the region (committed, a
readable protection, not PAGE_GUARD). When a runtime facility has an
arm per OS, check that Windows' arm is not an empty `{}` before blaming
the data it reads; print the raw values first (pc against the DWARF
ranges, the registers) to see which side is wrong.
