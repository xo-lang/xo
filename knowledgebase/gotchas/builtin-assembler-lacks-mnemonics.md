# The built-in arm64 assembler lacks some instructions

Status: Observed (2026-10-25)

Symptom: `xo: assembler: line N: unsupported instruction: ubfx x9, x27, #32, #8`
(also `sxtb x9, w9`) from a template in compiler/arm64 that the emitter wrote
into the assembly; the build fails at the assembler, not in the compiler.

Cause: compiler/macho (op.xo, enc.xo) encodes only the instructions the
emitter has used: loads and stores are `ldr`, `str`, `ldrb`, `strb`, `ldrh`,
`strh`, `ldrsw`, `ldur`, `stur` (no `ldrsb`, `ldrsh`); `ubfx`, `sbfx` and the
`sxtb`/`sxth` forms are not encoded (`sxtw` is). The text is also what
`xo asm` prints, and the clang based paths accept all of them, so a template
can look right and still fail only on the Mach-O assembler.

SIMD loads and stores, before writing a loop (last verified: 2026-11-07):
`ldp`/`stp` of q, d, x or w registers take an offset, a pre index and a post
index; `ldr`/`str` of a q register take an offset, a register offset (no
shift), and since 2026-11-07 a pre or post index (`ldr q29, [x17], #16`
failed rt.mem_skip_eq32's first gate); not encoded at all: `ld1` to `ld4`
and `st1` to `st4` in every form (multiple registers, one lane, `ld1r`),
`ldnp`/`stnp`, and `ldp`/`stp` of s registers. Load 64 contiguous bytes as
two post-indexed `ldp q` pairs.

Fix: spell the operation with supported ones: `lsr` then `and` for `ubfx`,
`lsl` then `asr` for a sign extension of 8 or 16 bits, `ldrb`/`ldrh` then that
for `ldrsb`/`ldrsh` (decisions 0369 and 0370 do). Or add the instruction to
op.xo and enc.xo with a test in macho_test.xo.
