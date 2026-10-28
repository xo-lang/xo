# sort_by with a tuple key of a Str, and a map's list appended through get

Observed 2026-10-28 (xo-lang-6, insert_rc). Status: Observed.

**Symptom.** `out.sort_by(fn(l) { (l.root, l.path) })` over the live
leaf sets of reference counting insertion was 7.2 s of a cold
self-build's 174.7 s CPU (4 percent of the whole compile, 17 percent of
insert_rc), although the sets are small and the comparison trivial.
`var p = c.pre.get(to) ?? []; p.push(v); c.pre[to] = p` was another
1.6 s.

**Cause.** `List.sort_by` builds a key list (each key a tuple holding
a retained `Str`), an index list and the merge sort's lists, all grown
by push, then gathers the elements: five allocations that grow and a
move of every element twice, per call; comparing the tuples compares
the strings. A list read out of a map and pushed to is copied whole
(copy on write: the map still holds it) on every append.

**Fix.** Sort an `Int` key with `List[Int].sort` (the pdqsort) and
gather: here `(root * R + rank(path)) * n + i` with the paths' ranks
kept beside the interned paths (compiler/ssa rc.xo `path_ranks`,
`sort_leaves`), no string compared. An insertion sort of the equal-root
runs by path was tried first and was quadratic (21 s): a struct of many
heap fields makes long runs. For the map's list, `remove` it first (or
set the entry to `[]`) so the push does not copy. Measure with the
cold self-build profile (`XO_PROF=cpu`, caches off) and `time -l`
instructions, which hold under load where wall does not.
