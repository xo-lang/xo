# amd64 ties: the tied operand left in the operation's uses

Status: Observed (2026-10-17)

## Symptom

The amd64 retarget wrote a copy (movapd, mov) before nearly every two
operand operation, even when the copied operand died there: nbody's
inner loop was a third copies, and making commutative operations tie
the dying operand (b73ac8e1) changed almost nothing.

## Cause

x86_retarget turns arm64's `op d, a, b` into a K_PMOVE `d <- a` (the
tie) and the operation `op b, d`, with d appended to the operation's
uses. The operation kept `a` in its uses too, though its text names
only b and d: liveness saw `a` read after the copy, so its range
overlapped d's and regalloc's coalescing (moves whose ranges do not
overlap share a register) skipped the pair. Only checked operations
whose fault stubs recompute the operand (x86_untie_recovered) had
dropped it.

## Fix

x86_untie_recovered now runs for every tie without a fault (and for
the carry chain links of x86_split): the tied operand is zeroed in the
uses wherever the text does not name it. `a op a` is written
`op d, d`, so `a` dies at the copy as well. A faulting operation keeps
the operand: its stub may read it. Measured in
bench/results/compare-2026-10-17.md (nbody 0.770, collatz 0.872).
