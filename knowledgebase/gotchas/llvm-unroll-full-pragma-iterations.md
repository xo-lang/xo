# llvm.loop.unroll.full unrolls up to a million iterations

Status: Observed (2026-10-22, decision 0338's first gate)

Symptom: with the loops with Float arithmetic marked
`llvm.loop.unroll.full` and `-pragma-unroll-threshold=300`, nbody and
xo's own build were fine, but clang's compile of a ten line dot/axpy
test program ran over 13 minutes (cc1 at full CPU, killed). The same IR
without the mark on one loop compiled in 4 s.

Cause: the marked loop was main's outer loop, `while r < 20000` with
`t += dot(a, b)` in it: a known trip count of 20,000 around two loops
inlined into it. A loop marked `unroll.full` with a known trip count is
handled by LLVM's pragma path, whose iteration limit is
`-pragma-unroll-full-max-iterations` (1,000,000 by default); the size
limit `-pragma-unroll-threshold` did not keep clang from the work.

Fix: pass `-mllvm -pragma-unroll-full-max-iterations=32` with the
threshold, to the compile and to the LTO link (compiler/xo split.xo
unroll_flags). Marking only innermost loops instead lost nbody's gain
(2,305 M against 1,160 M: its outer 5 body loop must unroll too).

Also: when testing a clang hang, a `perl -e 'alarm N; exec @ARGV'
clang ...` wrapper stops only the driver; its cc1 child keeps running.
Add `-fintegrated-cc1` so the compile runs in the process the alarm
stops.
