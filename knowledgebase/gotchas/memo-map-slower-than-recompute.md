# A per-value memo in a Map can cost more than recomputing

Status: Observed (2026-10-21, xo-fa)

## Symptom

Two insert_rc speedups that memoized a small List result per value made
no gain, or a loss, in paired cold self-builds (each output byte-identical):

- `leaves(f, v)` memoized in a `Map[(Int, Int), List[Leaf]]`, keyed by its
  base leaf and type: user CPU flat (170.3 vs 169.7 s, 174.4 vs 175.5 s).
- `cancel`'s per-value memos (`Map[Int, Leaf]`, `Map[Int, List[Int]]`
  of touched roots): cancel's own CPU went from 3.8 to 6.7 to 7.6 s
  (profiles), and `touches_any` cost 3.3 to 3.9 s against 1.4 for the
  `touches` it replaced.

## Cause

A memo hit is a hash of the key plus a copy of the stored List out of the
Map (Xo's maps hand out values, not references). The work it replaces was
a short walk (a value's base: a few Field hops) or a few list pushes. A
profile's cumulative time for a function (`leaves` 8.3 s) counts its
callees too, and most of that (`base`) still ran under the memo.

## How to avoid

- Before memoizing, split the profiled cost: flat time of the function
  itself versus its callees, and the call count. Memoize only when the
  part the memo removes is several map lookups' worth per call.
- Prefer a List indexed by a dense id (value index, leaf number) over a
  Map: no hash, and an Int or Bool element is not copied
  (insert_rc's verifier lists, 3c9f8385, did gain).
- Measure the function's own share in a profile of each build (old and new
  alternating), not total user CPU, which is noisy by more than a 5 s
  change under load.
