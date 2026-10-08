# Reading one field of a list element copied the whole struct

Status: Observed (2026-10-08, bench/perf/llvm nbody)

Symptom: nbody (a List of seven field Float structs) ran at 4.2x of Go
after List writes became in place; the profile put the time on lines
like `let dx = bs[i].x - bs[j].x`, not on the writes.

Cause: `bs[i].x` was generated as `bs.AtM(i, at).F_x`. Go keeps only
structs of at most four fields in registers (SSA decomposition), so the
inlined `AtM` result of a seven field struct is materialized in memory: a
56 byte copy per field read.

Fix: a field read of a List element uses `bs.RefM(i, at).F_x` (a
pointer to the element, std/rt/mono.go). Go may load the field after a
later call in the same expression (operand loads are not ordered with
calls), so the generator uses `RefM` only when the statement or condition
being generated cannot write the list (`fnGen.unit`, `mutates`). nbody
went from 0.68 s to 0.25 s.
