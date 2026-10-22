# Go backend: checked arithmetic on sized integers (2026-10-22)

Item 7 of bench/results/go-backend-gap-2026-10-22.md: bench/perf/portable's
arith8, u64, divmod, shift and arith32 ran 5 to 6x the native backend on
the Go backend.

Cause: `-gcflags=-m=2` on the generated program: the checked operations on
the sized types (UInt8 to UInt64, Int8 to Int32) are the generic
`rt.AddU`, `rt.MulU`, `rt.RemU`, `rt.DivS`, `rt.Shl` and friends, and Go
does not inline most of them (the shape copies cost 82 to 240 against a
budget of 80: a call to the fault formatter alone costs about 57), so every
`h % 251` of a UInt16 was a call. Int (64 bit) already had inlining
helpers (gotchas/checked-arith-inline-budget.md).

Fix: std/rt/sized.go, one non-generic helper per type and operation
(`Add_uint16`, `Rem_uint8`, `Shl_int32`, ...), written by
scripts/gen-rt-sized.py. Operands of 32 bits or less are widened to
int64 or uint64 where overflow is a range test; UInt64 uses bits.Mul64.
The fault is a panic of a `*sizedFault` (a panic costs the inliner about
one node), and the message and stack are built when it is recovered, as
for `*arithFault`. compiler/gen's `sized_op` emits them; a division or
remainder by a positive constant is plain Go. std/rt's
TestSizedHelpersMatchGeneric checks every operation of every type against
the generic function at the edge values (value and fault text); the Go
parity goldens check the fault output.

## Measurements

`xo bench --ab --rounds=5`, the same xo (this change's) building both Go
backend programs of bench/perf/portable (before: the generic helpers, after:
sized.go) and the native one (arm64), argument the workload, a gate slot
on the development Mac (arm64). Instructions retired (the Go runtime's
other threads included; the `none` setup alone is 16.4 M native and 38.0 M
Go backend). Checksums are equal on both builds.

| Workload | Go before | Go after | after / before (instructions) | wall | native | Go after / native |
|---|---|---|---|---|---|---|
| arith8 | 246.0 M | 106.3 M | 0.432 | 0.814 | 50.3 M | 2.11x |
| u64 | 262.4 M | 108.1 M | 0.412 | 0.698 | 76.4 M | 1.42x |
| divmod | 843.1 M | 292.1 M | 0.346 | 0.406 | 154.4 M | 1.89x |
| shift | 476.1 M | 115.9 M | 0.243 | 0.460 | 92.3 M | 1.26x |
| arith32 | 344.4 M | 114.0 M | 0.331 | 0.768 | 74.4 M | 1.54x |
| conv | 500.1 M | 397.8 M | 0.796 | 0.776 | 158.9 M | 2.50x |

Every row's interval is a "better" verdict (n=5). Before the change the
ratios to native were 4.6x to 5.5x (3 rounds); what remains includes the
Go runtime's 22 M instructions more at startup than native's. `conv` was
not in the ranking: its Int to sized conversions already had helpers; its
gain is the `%` and `+` on sized values in the loop.
