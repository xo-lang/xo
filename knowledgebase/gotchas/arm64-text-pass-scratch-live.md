# An arm64 text pass that writes x16 or x17 can break a later instruction

Status: Observed (2026-10-18, Xo lang 4)

## Symptom

A first version of compiler/arm64 lsr.xo copy_runs rewrote each run of
word copies through x16 (`ldr x16, [sp, #a]` then `str x16, [x17, #b]`)
as `add x16, sp, #a; add x17, x17, #b` and q register pairs. The fixed
point and `-short` passed. A scan of the HTTP server's assembly found 6
runs followed by another `str x16, [x17, #c]` of the same object:
those stores would have gone to the moved x17. Separately,
TestDebugInfoSameCode failed: a debug line row label in the middle of
a run split it in the build with debug information only, so the two
builds' code differed.

## Cause

x16 and x17 are the emitter's scratch registers, but one SSA
instruction's code can keep a value in them across many lines (an
object's address in x17 while its fields are stored, a call's target
in x16 while x17 copies its stack arguments). A text pass sees only
lines, not where an instruction's code ends. Debug row and variable
location labels (`_dl<n>`, `_dv<n>`) sit between instructions only
when debug information is on.

## Fix

Before a text pass writes x16 or x17 where the original code did not
(or leaves them holding something else), check that each is written
before it is read in the lines after (lsr.xo scratch_dead: a call, a
return, an unconditional branch or a block's label also ends it; a
numeric local label `1:` or a branch to one does not). Pass over debug
labels with past_rows (or label_def plus is_row_label) and keep them
before the rewritten code, as pair_mem does. Test such a pass on a
large program's assembly (`xo asm`) for the patterns it must leave
alone, not only with the fixed point.
