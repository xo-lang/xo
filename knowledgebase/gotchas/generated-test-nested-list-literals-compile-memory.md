# A generated test of nested list literals took 33 GB to compile

Status: Observed (2026-11-16)

## Symptom

TestHeapFuzz (internal/stdtest/heapfuzz_test.go, task #98) wrote a
`_test.xo` with 3,000 cases, each a tuple of `List[Int]` literals (an
initial heap of up to 200 values, up to 120 operations as `(Int, Int)`
pairs, the answers, the final order). `xo test` on it grew past 33 GB
while compiling and heavy.sh stopped the job. The same cases as 300 rows
compiled, slowly.

## Cause

Every element of a constant `List[...]` literal is an expression the
compiler types, lowers and emits on its own; a few hundred thousand nested
Int literals in one constant make a function body that size, and memory
grows with it. A generated corpus multiplies it by the case count.

## Fix

Encode a case's lists as text and parse them at run time: one `Str` per
list ("3 1 4", "0:5 1:0" for pairs) and small parsers in the generated
file (`ints`, `ops_of` in heapfuzz_test.go's check code). The corpus is
then a `List[(Str, Str, Str, Str)]` of string literals, which compile in
seconds at 3,000 cases. Other fuzz generators with list-valued cases
(deque, bufio, rate) were written this way from the start.
