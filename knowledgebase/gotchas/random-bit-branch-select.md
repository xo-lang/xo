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


## cond_inc (task #446, 2026-11-19)

The arm64 isel pass cond_inc lowers `if c { k += 1 }` to `cset`/`csinc`
plus the add for any condition (it does not go through from_memory: the
side block holds only the k + 1).

- Symptom: text float_fmt 1.011 of the branch build (1.008 to 1.018,
  host 1, 200 repeats) with equal callgrind counts (1,911,650,667 against
  1,911,651,222). Cause: code layout. The sites cond_inc changed before
  std/native num moved every hot function (num.layout from offset 32 to
  48 mod 64); a branch build padded by 16 bytes in main to the same
  offsets against the cond_inc build gave 1.003 (0.991 to 1.013, within
  noise). Check: before blaming a codegen change for a 1% loss with equal
  instruction counts, compare against a padded build at the same offsets
  (go tool nm, offset mod 64).
- A checked add where the branch build had none: num.dbox's rounding
  `xi += 1` had bounds proving no overflow, and cond_inc first always
  emitted `adds` and `b.vs`. cond_inc now asks safe_add as the Add
  lowering does (codegen test condinc round_up).
- A single 5 ms run reads "too short": measure text and random cases with
  a repeat count (float_fmt 200 times, random 20 times).
- `&&` of two compares (`if x > -1.0 && x < 1.0 { k += 1 }`, random's
  normal at 25%) is two blocks in SSA, so cond_inc never saw it; the
  branch on the first compare mispredicts as often as a single random
  one. and_inc lowers it as `cset`, `cset`, `and`, add (setcc, movzbl,
  andq on x86-64): bench/perf/random normal 0.638 of the time on
  linux/amd64 (4 rounds, 286e38ba8 against eccbfe06c). In and_inc, test
  the second block's compare with `g.has_val`, not `g.lazy`: lazy is set
  only once a block is selected, and the second block never is.
