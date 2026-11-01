# 0450: Small size classes carved from spans, so freed memory serves any class

Status: Proposed (xo-lang-2, 2026-11-01; awaits the owner). The proposal is
not to build spans: the measurements below find no footprint for them to
return.

## Context

Decision 0448's emit peak was first read as allocator retention. After
decisions 0448 stage 3 and 0452 the Mac's self build peaks near 4.3 GB
(the counter), with about 2.8 GB of 4 MiB chunks and about 1 GB of big
blocks. This decision asked whether the free memory inside the chunks can
be returned. Every figure is from the Mac's self build at
XO_EMIT_AT labels 24 and 40 (parts done, around the peak), from probes
never pushed, one build per row unless stated, so differences of a few
percent are not resolved. Linux figures are stated as such.

What is free (central magazine chains walked under their locks, the lists
and spares counted):

| Bytes | Part 24 | Part 40 |
|---|---|---|
| 16 to 96 B classes | 384 MB | 454 MB |
| 112 to 256 B | 324 MB | 345 MB |
| 272 to 1024 B | 52 MB | 77 MB |
| 2 KiB | 25 MB | 36 MB |
| 4 KiB | 42 MB | 70 MB |
| 8 KiB | 19 MB | 139 MB |
| 16 KiB | 41 MB | 220 MB |

The mid rows agree with a balance of counters (carved, plus received from
cuts, less live and cut): 22, 41, 21, 42 and 29, 68, 140, 218 MB. The
chain walk finds 1100 and 1238 MB on the central lists, more than
magazines times capacity gives (about 865 and 1050 MB with the lists and
spares). The likely cause, not verified: `recount` can make one long list
of the single threaded phase one oversized magazine.

The mid classes are not the gap. Their blocks are about 60% filled
(requested bytes over class bytes, cumulative: 0.61, 0.60, 0.59, 0.66 for
2 to 16 KiB), but they hold only about 126 MB free at part 24, with mapped
memory already at its level at part 40. XO_HEAP_MID_MAX=4096 lowers the
Mac's peak about 10% (4333, 4366 to 3903, 3817 MB) at a wall of about 1.1
times; where that saving comes from is not established.

## What was tried

| Experiment | Result |
|---|---|
| List grow rounds capacity up to its block (Go's roundupsize) | Mac peak 4281, 4381 to 4228, 4216 MB; max RSS 4557, 4647 to 4504, 4500 MB |
| The cut (decision 0383 (a)) takes 16 KiB page blocks | linux/arm64 `xo bench --ab`, 3 rounds: wall 1.001, CPU 1.004, max RSS 0.996, all within noise. Mac, 2 builds each in a loaded window: peak unchanged, footprint 4.45, 4.42 to 4.41, 4.36 GB |
| Lower mid_max on linux/arm64 | counter 6175, 6351 to 3844, 3818 MB at 4096, max RSS 4.9, 5.4 to 4.2, 4.2 GB, wall about 2.5 times |

The 16 KiB class is already given back on darwin (free_mt's
`pg && scavenge_pages()`, decision 0448 stage 1), which likely explains the
cut's result there. Linux's max RSS barely moves with the counter, so any
claim from this work would be the Mac's peak and footprint only.

## The measurement that decides

Spans return a page only when every block on it is free. The scan adds up
the central magazines' free bytes per 16 KiB page. Today's bump carve mixes
classes on a page, so the first row is not a fair bound; the second carves
each small class from a 64 KiB run of its own, which is the layout spans
would have, with no release path.

| Layout | Part | Central free | Whole 16 KiB pages | Of those, the 16 KiB class at most | Pages 3/4 free |
|---|---|---|---|---|---|
| Today | 24 | 1100 MB | 276 MB | 37 MB | 476 MB |
| Today | 40 | 1238 MB | 306 MB | 221 MB | 563 MB |
| Per class runs | 24 | 857 MB | 27 MB | 43 MB | 299 MB |
| Per class runs | 40 | 1238 MB | 298 MB | 219 MB | 638 MB |

Separating the classes does not make whole pages. Net of the 16 KiB class,
already returned on darwin, per class runs leave about 0 to 80 MB, under 2%
of the peak. The free small blocks sit among live ones. The cut still
hands mid blocks to small classes in this probe, a mix a real span layout
would not have, and it is one build per row: the bound is rough, not exact.

## Decision

Proposed: do not build spans. The allocator work they need (a hot path
change, a release path, list unlinking) has a measured reach under 2% of
the peak. The peak is lowered by holding less live data (decision 0448's
next item, a slimmer ssa.Value).

## Consequences

The heap keeps its prefixed small blocks, cuts and scavenge. The free small
memory, about 0.8 GB at the peak, stays resident until the live data around
it shrinks.

## Alternatives

- Spans with 256 KiB runs or larger: not measured. Longer runs group more
  allocation times together, so more live blocks per run are expected, not
  fewer whole pages.
- Lifetime grouping (arenas per function in flight): frees whole regions
  together, which the scan says size classes do not. Decision 0435's front
  end arena measured no footprint win (its peak is the SSA plus emit's
  transients), so this would have to cover the SSA and emit, a different
  and larger arena. Not measured; a candidate after the slimmer ssa.Value.
- Finer mid classes (Go's, 4 per doubling): reach limited by the live mid
  blocks, about 680 MB at part 24, so about 200 MB at best by the fill above.
  An estimate, and a change to the heap record's layout. Not built.
- List roundup: its own linux `--ab` decides; a small gain if it holds.
