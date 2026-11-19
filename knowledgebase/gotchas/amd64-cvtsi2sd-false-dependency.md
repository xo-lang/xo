# amd64: cvtsi2sd waits for its destination's last writer

Status: Observed 2026-11-19 (task #407).

## Symptom

Keeping Floats in xmm registers across calls (call_saves for Floats)
made bench/perf/gomath faster on linux/amd64 everywhere except erf and
erfc, which became 4 to 15% slower than with every Float spilled, though
the loop had fewer instructions and no stack traffic.

## Cause

`cvtsi2sd` (and `cvtsi2ss`, `cvtss2sd`, `cvtsd2ss`, `sqrtsd`, `roundsd`)
writes only the low lane of its xmm destination, so the CPU makes it
wait for the register's previous value. With the new allocation the
loop's `Float(i)` landed on a register that the previous iteration's
long `divsd` chain wrote last, so iterations no longer overlapped.
Before, the values sat in stack slots and the register held a short
lived temporary.

## Fix

The x86 lowering of `scvtf` and `ucvtf` zeroes the destination first
(`xorpd d, d`, `x86_break_dep` in compiler/arm64/x86.xo), as Go and LLVM
do (codegen pattern in test/codegen/floats). The FP-source forms are not
zeroed: after allocation their source may be the destination. When a
register allocation change slows a Float loop on amd64 alone, look for
a partial-write instruction whose destination was last written by a
slow chain.
