# A struct's heap field count is paid by every copy and every drop

Status: Observed (2026-10-26, the guard fact bounds proof,
bench/results/sortip-swap-checks-2026-10-26.md).

## Symptom

Adding fact bookkeeping to the arm64 bounds proof grew xo's own native
`__text` by 215,336 B (+0.352 percent) while the new code itself was a few
hundred lines. The growth was spread over about 50 arm64 functions that
never call the proof: arm64.G.emit +11,432 B, its disassembly +2,858
instructions with the same `bl` count and +765 branches, and
arm64.G.seh_end +786 instructions. seh_end's growth is 60 copies of the
same sequence, one per G value it copies or drops: `tbnz`/`tbz` (is the
field present), `stadd` (count it up), a release branch (count it down and
free).

## Cause

The proof struct is a field of the arm64 generator (G.proof), and it grew
from two heap fields to five: a List of dominators and a Map of less facts,
plus new Maps of le facts, per container lengths and unstable containers.
Values have value semantics, and a copy or a drop counts every heap field
through nested structs (spec 5.1), not the size of what the field points
at. So each of the three new Maps added a retain and a release at every
site in every method that copies or drops a G, most of which have nothing
to do with the proof.

## Fix

Keep the field count small. Every kind of fact now lives in one map under a
key per kind (`fact_key(kind, id)` in compiler/arm64/fieldlicm.xo, kinds
LT, LE, LEN, UNSTABLE over one `Map[Int, List[(Int, Int)]]`), so Proof has
two heap fields again and the cost fell from +215,336 B to +8,696 B.

Grouping the maps into a sub-struct does not help: the count is taken
through nested structs, so a `facts: Facts` field with three Maps still
costs three.

## How to check

Build the change's own compiler to its fixed point's s2 and take sizebreak's
`__text` beside the base commit's, with one file set per variant, then
compare per symbol: `scripts/bootstrap-xo.sh SEED OUT`, `sizebreak` on
`OUT/s2/xo`, and `otool -tV` on both binaries to see the added
retain/release sequences. A grower that never calls the new code is the
tell.
