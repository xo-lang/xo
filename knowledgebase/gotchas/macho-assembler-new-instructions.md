# A new instruction in the arm64 emitter needs compiler/macho's assembler too

Status: Observed (2026-10-16, Xo Lang 3 and xo-fa; again 2026-11-03 for an arrangement, below).

## Symptom

`xo build --backend=arm64` on darwin/arm64 fails for every program that
reaches std's TLS: `xo: assembler: line 39952: unsupported instruction:
adcs x3, x13, x8`. `--as=cc` builds it.

## Cause

dd3b3e2d made the arm64 emitter write adcs and adc (cpu.adc's carry
chains, decision 0231 addendum); compiler/macho's assembler (enc.xo), the
default on darwin when xo links the program itself, had no encodings
for them. The goldens did not fail while programs still linked C (the
runtime's xo_rt.c), which took cc's assembler; once the C runtime was
deleted (3262260e) every program took macho's.

## Fix

enc.xo encodes adc, adcs, sbc, sbcs (registers, x and w), with their
cases in macho_test.xo. A change that makes the emitter write an
instruction it never wrote before adds its encoding and a test case
there in the same commit.

## A new arrangement of a known instruction (2026-11-03, Xo Lang 1)

The same holds per arrangement: op.xo's simd_word encodes each
mnemonic's arrangements one by one, so an `add vD.2d` failed ("line
6261: unsupported instruction: add v29.2d, v24.2d, v16.2d") although
`add` on .4s and .16b was known. rt.sha512_block's sequence (ssa
sha512hw.xo) hit it: the LLVM golden and Go parity passed, every native
arm64 build that reached SHA-512 failed (TLS, http; the fixed point's
TestModuleCheckAuto). Check each new line of an emitter sequence
against macho_test.xo's ENCODINGS, arrangement included, and add a row
from clang's word for any that is missing.
