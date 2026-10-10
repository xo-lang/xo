# A huge function fails to assemble: tbz and tbnz reach only 32 KiB

Status: Observed (2026-10-09, internal/arm64, TestTimeDiff)

Symptom: `xo build --backend=arm64` of a program whose `main` held a few
thousand list elements in literals failed in the assembler with hundreds
of `error: fixup value out of range` on lines like `tbnz x9, #63,
Lf5_b26800` (the LLVM backend built the same program).

Cause: `tbz` and `tbnz` have a 14 bit word offset, so they reach only
32 KiB (8192 instructions) either way. The backend uses them for the
reference count tests of every retain and release (`tbnz #63` to the
shared path, `tbz #62` past the atomic one), whose targets are cold
blocks at the end of the function; in a function over 8192
instructions the early ones cannot reach. `cbz`/`b.cond` (1 MiB) and
`b` (128 MiB) were not the problem.

Fix: `internal/arm64/emit.go` (`farTests`) counts the instructions of
each emitted function; past 7000 it rewrites every `tbz`/`tbnz` as the
opposite test over an unconditional branch (`tbz x9, #63, Lf5_far1; b
L; Lf5_far1:`). Functions under the limit are unchanged, so ordinary
code keeps the one instruction test. If a function ever passes 1 MiB of
code, `b.cond` and `cbz` need the same treatment.
