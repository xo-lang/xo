# A division by a constant as a multiply high retires more instructions on arm64

Status: Observed (2026-10-12, internal/arm64 and compiler/arm64, bench/perf/text, decision 0194)

Symptom: lowering `x / c` and `x % c` to a multiply high and shifts
(decision 0194) made bench/perf/text's Duration text faster in wall time
on the Apple M series machine but raised its instructions retired, the
metric of decision 0180's bar: 149,470,312 to 161,257,454 on arm64
(bench/results/const-div-2026-10-12.md, "Steps").

Cause: arm64's `sdiv` is one instruction (slow: several cycles of
latency), while the multiply high form is `smulh`, `asr`, and usually a
sign fixup `add`, plus a 64 bit magic constant that takes four
instructions (`movz` and three `movk`) wherever it is not hoisted out of
a loop. A count of instructions sees only the extra instructions, not
the shorter latency. Two smaller causes made it worse at first: the
quotient shared by `x % c` and `x / c` was memoized per MIR block only,
and a division that may fault ends its SSA block, so the two were in
successive blocks and multiplied twice; and every Int division keeps the
sign fixup unless the value ranges prove the dividend not negative.

Fix: the quotient memo (`memo.quo`, `Memo.quo`) is inherited by
dominated blocks like the other memos; text.xo masks the dividend
(`v & 0x7FFFFFFFFFFFFFFF`) so the ranges drop the fixup. Read wall time
next to instructions when judging a lowering that trades instruction
count for latency, and expect constant materialization to dominate in
short loops and straight line code. Next: logical immediate (`orr`)
materialization of repeating constants such as 0x6666666666666666 (one
instruction, plus a `movk` for the magic's low chunk).
