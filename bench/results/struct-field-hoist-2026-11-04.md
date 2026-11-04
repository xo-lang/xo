# hoist_fields for struct fields and `var self` calls (2026-11-04)

Branches lang1-dfl (struct fields) and lang1-fsum (per-callee `var self`
summary, ssa self_sets), Xo Lang 1. The question (lead, 2026-11-04): does
reading Deflater's fields once before deflate's loops close the gap Lang 3
saw in bench/perf/archive zip, so their hand-made local copies are not
needed?

## What changed in the code

- dfl (70a60e9b0): hoist_fields also reads once, in the preheader, a field
  of a struct the loop never changes (a borrowed self, or a `var self`
  whose other fields the loop sets). Deflater.find_match now loads
  d.hash_prev and d.hash_offset once (`ldp x23, x20, [x0, #152]`) instead
  of at every step of the hash chain.
- fsum (84c2bfce6): a `var self` passed through a method call in the loop
  keeps the fields that method never sets. deflate()'s loop goes through
  write_block and insert_hash; it now reads window_end, max_insert_index,
  sync, base, fast_skip, hash_offset, lazy and nice once before the loop.
  The walk needed 24 steps from the back edge to the header (a field set
  per step), so struct_root memoizes each block parameter and goes 32 deep.

## Measurements (linux/arm64, `xo bench --ab`)

| change | row | B/A | range | verdict |
|---|---|---|---|---|
| dfl against 0dcb7d631 | archive zip | 1.004 | 0.993 to 1.004 | within noise (n=3) |
| dfl | self build | 1.003 | 0.988 to 1.006 | within noise (n=7) |
| dfl | json | 0.996 | 0.992 to 1.021 | within noise (n=3) |
| fsum against dfl (bce6a0c76) | archive zip | 0.993 | 0.978 to 1.007 | within noise (n=5) |
| fsum | self build | 1.001 | 0.999 to 1.004 | within noise (n=7) |

Code size: xo `__text` 33,070,212 (dfl) to 33,073,252 (fsum), +3 KB.
s2 and s3 identical on both; amd64 sample 51 ok on both.

## Verdict

No measurable gain: with find_match's fields and then deflate()'s eight
read once before their loops, zip stays within noise (1.004, then 0.993),
and nothing else moves. The loads were cheap where they were (L1 hits
beside the loops' real work, the hash chain walk and the byte compares),
so both branches are parked (lead, 2026-11-04: push only with a real zip
win), not merged: lang1-dfl (bce6a0c76) and lang1-fsum (84c2bfce6 and its
note commits). Lang 3's hand made local copies in std/compress are not
needed for speed either: they would remove the same loads.

The full internal/arm64 suite passes on fsum (960 s); s2 and s3 are
identical; the amd64 sample is 51 ok. A first full run of dfl failed 13
tests beside fsum's suite on the same host (memory; all pass alone, see
gotchas/two-full-arm64-suites-one-host.md).
