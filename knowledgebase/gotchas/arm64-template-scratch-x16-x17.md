# An arm64 template that writes x16 or x17 corrupts reloaded operands

Status: Observed (2026-11-16, task #104; again 2026-11-17, task #103; 8b5f784fb, fixed by the commit
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

## Again on darwin (2026-11-17, task #103 lever 5)

rt.bytes_find_byte's scan template (calls.xo) and rt.mem_copy_block
still use x16 and x17 as their pointer and count. Once strings.replace
inlined the one byte scan, hoist_consts' six loop constants pushed the
scan's start into a spill slot; it was reloaded into x16, `add x16, $u0,
$u1` overwrote it, `sub x17, $u2, $u1` read an address, and
`s.replace(" ", "")` returned its input. Bisecting passes named
hoist_consts, which only raised the register pressure.

Fix: emit_plain now reloads the spilled uses (and places the spilled
definitions) of a template that names x16, x17, w16 or w17 past the two
scratch registers, in a caller saved register free at the instruction
(scratch_reg's third choice). Golden one_byte_replace_spill printed its
inputs unchanged before. A bisect-pass result that names an
optimization is a lead, not a cause: read the instruction that reads
the wrong value and the register's last write before it.
