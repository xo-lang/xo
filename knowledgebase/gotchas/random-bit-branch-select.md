# A branch on a random bit: fix the source, not select_branch

Status: Observed (2026-11-19, task #87)

Symptom: std random.normal ran 1.7x of Go on linux/amd64 (1.3x on
arm64) with equal instruction counts (callgrind). `if w >= 2^31 { w -
2^32 } else { w }` and `if j < 0 { -j } else { j }` test a random bit, so
both branches were mispredicted half the time; Go's compiler emits two
conditional moves.

Cause: select_branch (compiler/arm64/isel.xo) leaves a branch whose
condition comes from a load or call (from_memory) a branch, and spec_ok
would not speculate a checked Int subtract or negate.

What did not work: narrowing the from_memory veto to selects whose value
goes to the next iteration (a loop header join, or a header edge passing
a join parameter), plus speculating checked add, subtract and negate when
bounds prove no overflow. normal got 0.76 of the time, but the text
bench's selects went from 1155 to 1367 and predictable tests on loaded
values became slower: bench/perf/text float_fmt 1.025 (1.018 to 1.033),
zone_offset 1.039 (1.019 to 1.052), `xo bench --ab` on host 1 with the
workload repeated 200 times (a single 5 ms run read "too short" and
hid it). The compiler cannot tell a random bit from a predictable test.

Fix: write the hot code branch free where the bit is random
(a0d695aab): `j = w - ((w & 0x80000000) << 1)`, `m = j >> 63; aj = (j ^
m) - m`. bench/perf/random normal 0.782 of the time on host 1, amd64
1.76x to 1.38x of Go. Measure a codegen heuristic's losers with a
repeated workload (`--ab` runs under 10 ms say "too short").
