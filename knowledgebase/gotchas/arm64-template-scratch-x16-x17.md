# An arm64 template that writes x16 or x17 corrupts reloaded operands

Status: Observed (2026-11-16, task #104; 8b5f784fb, fixed by the commit
that added this file)

## Symptom

Every program built for linux/arm64 with `XO_ARM64_REGS=2` (the spill
test, internal/arm64 TestGoldenRunSpill/2) died with SIGSEGV before
printing anything; REGS=4 and darwin passed, so the Mac's gates never saw
it. The full suite on main failed 12 goldens.

## Cause

8b5f784fb made linux's read modify write atomics inline: a load of the
byte xo_lse into w17, then LSE or a load/store exclusive loop with x16 as
the sum and w17 as the store's status. x16 and x17 are the emitter's
scratch registers: it reloads spilled operands into them (emit.xo
scratch_reg). With two registers, the address and the xo_lse address
spilled and were reloaded into x16 and x17, and the template's own
writes overwrote them.

## Fix

A template's own registers are allocatable ones listed in Inst.clobbers
(calls.xo atomic_def_cl: x14 and x15), so no value is used, defined or
live in them at the instruction; never x16 or x17 in a template with
operands. test/codegen/atomics checks "w15" and -"w17" on arm64@linux.
A change to an arm64 template on a linux-only path is checked on linux
with XO_ARM64_REGS=2 (a linux/arm64 build run in Docker on the Mac
reproduces it: `docker run --platform linux/arm64`).
