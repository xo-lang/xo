# Debug lines are packed: line * 4096 + column

Status: Observed (2026-10-21)

## Symptom

TestBackendCacheEditIdentity failed for linux/arm64 after a statement
was added to one function: the build through the caches differed from
the cache-off build, and only in `.debug_line` and `.debug_info`. A
caller's rows for the moved functions it inlined kept their old line
numbers, with columns off by the number of lines added (line 1210
column 6 where the cache-off build had line 1213 column 3).

## Cause

`ssa.Value.line`, `DVar.line`, and `DBind.line` are packed: `line *
4096 + column` (compiler/ssa ir.xo). The front cache's relative lines
(front cache v12, decision 0271 addendum) shifted them by adding the
line delta to the packed value, so the delta landed in the column. The
positions (`file:line:col` strings) shifted correctly, so fault
messages were right, and a Mach-O executable holds no DWARF (its rows
are in the dSYM), so the host builds the tests compared were equal.

## Fix

shift_lines adds `delta * 4096`, and relocatable compares `line /
4096` with the declaration's line (front cache v13). Any code that
moves or compares these fields works on the unpacked line.
TestFrontCacheMovedBodies now also builds linux/arm64, whose executable
holds the line table.
