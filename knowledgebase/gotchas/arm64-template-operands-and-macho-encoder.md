# arm64 instruction templates: ten operands, and macho must encode them

Status: Observed (2026-10-16, decision 0231's addendum, adc chains).

Symptom 1: an arm64 build fails in the assembler with `adcs x24, x260,
x261` (invalid operand). Symptom 2: with the default assembler and
linker (`--ld=macho`, compiler/macho), `xo: assembler: unsupported
instruction: adc x20, x26, x25`, while `--as=cc` builds and runs.

Cause 1: the arm64 emitter's instruction templates name operands with
one digit ($u0 to $u9, $d0 to $d9); `$u26` reads as $u2 then a literal
6. A chain of seven adc pairs (P-256's square doubling its cross
products) has fourteen uses. Cause 2: compiler/macho's encoder only
knows the instructions someone added to it; adc and adcs were new
(0a45b2b1 added them, with sbc and sbcs).

Update (2026-10-16): templates read operand numbers of any length now
(emit.xo operand_num, opt.xo shift_uses), so Cause 1 is gone; the
cut below stays until adc_chain is switched back on
(arm64-template-scratch-reuse.md).

Fix: compiler/arm64 calls.xo adc_chain cut a chain at five pairs (chains are now off: gotchas/arm64-template-scratch-reuse.md)
(four after a `cmp #1`), the next instruction taking its carry in from
the previous one's cset. A new instruction an emitter writes goes into
compiler/macho enc.xo in the same commit, and the gates build with the
default assembler and linker (not only `--as=cc`). Also: a template of
several instructions that writes a def before reading every use must be
marked `early`, or the register allocator may give the def a register a
later line still reads (every arm64 TLS handshake failed until the
adc chain was).
