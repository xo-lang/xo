# arm64: a near frame's stack parameters past sp's reach

Status: Observed (2026-10-22, xo's self-build with every optional pass off)

Symptom: `XO_PASSES` turning every optional pass off made xo's own
arm64 build fail in the assembler on `str x17, [sp, #32768]` (the
scaled 12 bit offset of `ldr`/`str` reaches 32760). Pass bisection over
the self-build was blocked by it.

Cause: regalloc chooses a far frame (x28 kept free, far_slots rewriting
every sp relative access past its reach, decision 0146) only when the
frame's bound passes MAX_FRAME (28 KiB). MAX_FRAME leaves 4 KiB of the
reach for the function's own stack parameters and results, which sit
past its frame in the caller's (`[sp, #fr.size + 8 * slot]`). Nothing
checked that 4 KiB: a frame near 28 KiB whose stack parameters or
result words take more than the rest addressed them past 32760.

Fix: `in_arg_bytes` (compiler/arm64 emit.xo) measures that area, and
regalloc takes a far frame also when the frame's bound plus it passes
MAX_FRAME + 4096; far_slots then reaches those words through x28 like
the far slots.
