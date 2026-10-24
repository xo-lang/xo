# Equal instruction counts can hide cold reference count writes

Status: Observed (2026-10-24)

## Symptom

`List.sort` of 200,000 Strs ran 1.18 of Go's wall time with 1.001 of its
instructions (bench/perf/sort strs). Ints, with the same algorithm, matched Go.

## Cause

The generic merge moved Strs: each `b[k] = a[i]` retained the string it
copied and released the one it replaced, a read-modify-write of two cold
string objects per element per pass. An instruction counts the same whether
its line is in cache or not, so `--ab`'s instructions row stayed equal while
wall time and max RSS did not.

## Fix and how to avoid it

Move words that carry no count: sort a list of indices keyed by a prefix and
gather the Strs once (std/native/prelude.xo list_sort_str, 0.894 of the old
wall). When wall is worse and instructions are equal, look for a count
update or a pointer chase per element in the disassembly (`otool -tvV`, a
load, add, store on `[x20]` before each move) before blaming the algorithm.
