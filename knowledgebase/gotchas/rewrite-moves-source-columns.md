# A source rewrite with the same code builds different binaries

Status: Observed (2026-10-18, Xo lang 4)

## Symptom

Decision 0260 step 3 rewrote std's `rt.int_wrap(K, a, b)` calls as
`a.wrapping_add(b)` and the like, which lower to the same intrinsics.
A check that the compiler and every internal/llvm golden build byte
identical before and after failed for all of them (390 of 390 golden
binaries and the compiler).

## Cause

The rewrite shortens lines, so source columns after it move. Columns
are in the build twice: the DWARF line program and inlined call sites
(`__DWARF` sections), and the `PATH.xo:LINE:COL` strings of fault
messages (`std/native/llrt/pool.xo:379:79` became `:76`). The code and
the rest of the data were the same.

## Fix

Compare the assembly (`xo build --asm`) with the DWARF sections and the
columns of location strings removed: `scripts/asmnorm.py FILE` prints
it, and two builds whose sources differ only in columns hash equal. For
a rewrite that also moves lines (one call split over two), drop the line
numbers too, or the check fails the same way. Say "the same code and
data", not "byte identical", for such a change.
