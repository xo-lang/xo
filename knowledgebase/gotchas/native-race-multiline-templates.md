# Native --race missed an atomic written as a template of several lines

Status: Observed (2026-10-20, Xo Lang 2, decision 0273 part 2).

Symptom: on the arm64 backend's --race, the race table's Atomic add case
reported a race although the add ordered the two tasks (both thread
counts); every other row, Atomic set and get included, passed. The
report named the program's two accesses and no atomic.

Cause: compiler/arm64 race.xo reads an instruction's memory access from
its template's mnemonic and address operand. Compare and swap is one
instruction of four lines (`mov $d0, $u0`, `cas... $d0, $u1, [$u2]`,
`cmp`, `cset`, compiler/arm64 calls.xo), so the pass read `mov`, found no
access, and added no `__tsan_release` before the `casal` that publishes
the add. Atomic.add (std/native/llrt atomic.xo) loops on that compare and
swap, so its release was missing while its loads' acquires were there.

Fix: race_line picks the one atomic line (ldar, stlr, an LSE read modify
write or compare and swap) out of a template of several lines and parses
that; a template of several lines with no atomic or more than one (the
crypto kernels) stays uninstrumented. Any new multi-line template that
holds an atomic is covered; one that holds plain loads or stores of the
program's memory is not, and needs its accesses listed by hand. Test:
internal/llvm TestRaceSyncTableArm64 (the atomic_add row).
