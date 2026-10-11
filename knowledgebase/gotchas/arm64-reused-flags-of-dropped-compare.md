# arm64: a block reused the flags of a compare that was never emitted

Status: Observed (2026-10-10)

Symptom: random integer programs (`progen.Arith`, TestArithProperty)
printed different values on arm64 than on the Go and LLVM backends:
`(if p0 == p0 { p0 } else { p0 }) - (if p0 >= p0 { 0 } else { p0 })`
was 0 instead of p0 when an overflow-checked `adds` ran just before.

Cause: `reuseFlags` (internal/arm64/opt.go) drops the compare of a
block whose single predecessor ends with the same compare of the same
registers. The first `if` has identical arms, so both of its targets are
the next block, and emit (iCondBr with `t == f` after `fwd`) wrote a
plain `b` and dropped its compare too. The second `if` then branched on
whatever set the flags last.

Fix: emit keeps the compare of a branch whose targets coincide
(internal/arm64/testdata/run/flags_same_targets). When a pass relies on
an instruction of another block, check every later pass that may remove
or rewrite that instruction, emit included.

Follow up (2026-10-11): the other passes that rely on other blocks
(threadConst, threadCset, selectCompares, placeLatches, and the memos
inherited from a dominator) were reviewed against every later pass and
emission; internal/arm64/crossblock_test.go breaks each assumption one
at a time and requires the pass to decline.
