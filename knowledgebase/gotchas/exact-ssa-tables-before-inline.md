# Exact-size SSA tables before inlining cost more than they save

Status: Observed (2026-10-31, xo-lang-2, ADR 0448 stage 3)

Symptom: ssa.renumber rebuilt each function's value table and its lists
at their exact size (List.with_capacity) instead of growing them by
push. Through cleanup and insert_rc the self build's heap was 70 to 240
MB lower, as meant, but from emit to the link it was 200 to 300 MB
higher, and max RSS (set at emit) rose 3 to 4% on the Mac in four tries
(in place, exact, exact without empty blocks, and a trim after
inlining).

Cause: in part, the arm64 inliner (inline_one) pushes the callees'
copies onto the caller's value table, so a table with no room left is
reallocated at twice its length at the first push, where a grown table
still had slack; the module keeps the tables through emit and the link.
Trimming each inlined function's table back to its size (a copy after
inline_one) moved the savings later but still left 307 MB more from emit
on, not explained (one guess: freed large blocks of exact, odd sizes are
reused less than the doubled sizes push makes).

Fix: none needed; renumber keeps growing its lists by push. Before
sizing a structure exactly, compare the per-phase heap (`--phase-times`
heap and peak columns, base against change) through emit and the link,
not only where the structure is built.

Same trap, earlier (xo-lang-6, 2026-10-30, the same renumber change on the
arm64 host: max RSS 8248 against 8249 MB): an exact list is a full list, so
the next push reallocates it and takes the slack back. Size a list at its
last writer, or copy it to its size after the last pass, or leave it. The
growth curve has changed since (decision 0452: doubling only under 256
elements), so re-measure these figures before relying on them.
