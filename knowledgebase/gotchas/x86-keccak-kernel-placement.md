# The unrolled x86-64 Keccak kernel is a third slower at some offsets

Status: Observed (2026-11-19, std sha3 on the linux/amd64 host, Zen 5)

Symptom: the same sum_once code and kernel (ssa.keccak_absorb_x86_lines)
hashed bench/perf/sha3 big in 56 to 62 ms in some programs and 82 to 88
in others. Stack depth (swept over 9 KB) did not matter; small edits to
sum_once or the bench program flipped a binary between the two speeds.
That flipping made two wrong causes look right before the pattern
showed: the operands passed through frame slots, and store to load
reloads of the state.

Cause: where the unrolled rounds (22 KB, about 5,300 instructions, no
loop) start in a 64 byte line. Fast binaries had them at 9, 11 and 25
bytes past a boundary; slow ones at 43 and 57. With `.p2align 6` before
the rounds every program ran 56 to 62 ms (Go 82).

Fix: x86keccak.xo aligns the rounds (`.p2align 6`; the long NOPs run
once a call), guarded by x86keccak_test.xo. When a big straight line
kernel's speed changes with unrelated edits, tabulate the kernel's
address mod 64 (objdump the binaries) against the times before
explaining it by the edit. Compare the Apple M4 loops, where alignment
made speed stable but not faster
([arm64-loop-alignment-stable-not-fast.md](arm64-loop-alignment-stable-not-fast.md)).
