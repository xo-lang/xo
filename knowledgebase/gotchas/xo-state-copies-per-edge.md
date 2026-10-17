# Xo: a dataflow state copied per edge costs more than the map it replaces

Status: Observed (2026-10-12, compiler/ssa verify.xo)

Symptom: replacing the verifier's `Map[Str, Int]` of reference counts
(the ordered Map records an entry per change) with integer leaf numbers
made the `--stages` run slower, not faster: a dense `List[Int]` per
state 1.6 to 3.0 times slower, a sparse sorted list no faster, the
sparse list with zero entries kept 1.3 to 1.8 times slower
(bench/results/selfhost-ssa-2026-10-10.md, 2026-10-12 section).

Cause: Xo values copy on write. Each block starts from a copy of its
entry state and each edge from a copy of the block's state, so the
first change after a copy copies the whole state (dense: every leaf of
the function; sparse: everything held, and every insert or removal then
shifts the list). The count of copies, not the key type, was the cost.

Fix: count in one reused dense list (a field of the checker), reset
through the list of entries it touched, and copy a canonical sparse
state out only where one must be stored or compared (once per edge),
undoing the edge's own changes after the copy. Memoize per value what
does not change between calls (leaf numbers, uses). In general: in a
port of a Go dataflow pass whose states are maps cloned per edge, look
at the number of copies before the representation.
