# 0455: A heap record's first chunks start at 256 KiB

Status: Proposed (needs the owner's acceptance, allocator change).

## Context

Decision 0448 case (c) wants a small build's mapped heap within 2 times live. Coll's warm build on the Mac (2026-11-02): counter peak 78 to 86 MB, about 26 MB live at fe.build, 9 MB at the link. Each thread that allocates takes a 4 MiB bump chunk and a 4 MiB page chunk; a count at exit showed 8 of 9 records with over 3.9 MiB of most chunks untouched, about 50 MB of the counter. The memory budget (decision 0349) is charged for it too. On Linux with THP `always` the first touch of such a chunk can fault 2 MB (coll edit RSS 121-125 default, 88 with THP off, on the second host).

## Decision

A heap record's first bump chunk and first page chunk map 4 MiB as today, aligned as is_page needs, with only the first 256 KiB accessible and counted; the rest is PROT_NONE, so no other mapping lands inside it and no huge page faults in. On exhaustion the chunk grows in place by four times its size (mprotect), and later chunks are whole. Darwin only; Linux, Windows and sanitizer builds keep whole chunks.

## Numbers (base 083fea894)

All chunks small, then first chunks only (decided):

| | All chunks | First only |
|---|---|---|
| Mac counter peak, coll warm (78-86 before) | 33-35 MB | 33-36 MB |
| Mac coll warm --ab wall | 0.991 | 0.988, 0.995 |
| Mac coll warm --ab instructions | 1.001 | 0.999, 1.000 |
| Mac coll warm --ab RSS | 0.996 | 1.003, 1.006 |
| Linux coll edit wall / RSS | 1.045 / 0.916 | 1.005 / 0.912 |
| Linux conc-spawn edit wall / RSS | 1.062 / 0.893 | 1.027, 1.025, 1.020 / 1.048, 1.027, 0.999 |
| Linux self wall / RSS | 1.041 / 0.994 | 1.003 / 1.008 |

On Linux first chunks only cost conc-spawn 2 to 3% and give it no memory, so Linux keeps whole chunks. Mac alloc --ab, 3 rounds: mixed wall 0.987, instructions 1.000, RSS 1.0; strings 0.992, 1.000, 1.0. Mac self build --ab, 3 rounds, backend cache off: wall 0.996, CPU 1.005, instructions 1.000, max RSS 0.989.

## Consequences

The counter and the budget read closer to real memory. Mac RSS and footprint do not move, so on darwin this is the counter's target, not memory. Up to two more mprotect calls per record per chunk kind.

## Alternatives

- Every chunk small: Linux self wall 1.041, it loses THP on the chunks that fill.
- Charging a chunk as its bump passes each MiB, with no mprotect: the counter alone, THP unchanged.
- First chunks only on Linux too: conc-spawn wall 1.020 to 1.027 with no RSS gain.
