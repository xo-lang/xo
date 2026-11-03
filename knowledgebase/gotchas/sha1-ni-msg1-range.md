# SHA-NI SHA-1: sha1msg1 runs through group 16, not 15

Status: Observed (2026-11-03, Xo Lang 1, rt.sha1_block's x86 lowering)

## Symptom

`cpu_sha1` on linux/amd64 printed a wrong digest whose last word (e) was
right and whose first four (a to d) were wrong; arm64 and the software
compression were right.

## Cause

In the SHA-NI schedule each group g starts group g + 3's words with
`sha1msg1`, adds group g + 2's xor with `pxor` and finishes group g + 1's
with `sha1msg2`. The last group whose words still need a start is 19, so
`sha1msg1` runs for g = 1 to 16 (pxor 2 to 17, sha1msg2 3 to 18). Stopping
`sha1msg1` at 15 leaves group 19's words without it: only the last four
rounds are wrong, and e is set before them, so e alone comes out right.

## Fix

compiler/ssa/sha1hw.xo sha1_ni_lines: `g >= 1 && g <= 16`. The arm64
form needs no such range (sha1su0 and sha1su1 run in groups 0 to 15).
A golden with a known digest (cpu_sha1) on the amd64 sample catches it;
the Mac's TestSHA1HWDiff covers only arm64.
