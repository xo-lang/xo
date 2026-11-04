# 0492: the large block cache keeps past its bound the sizes a program asks for again

Status: Accepted (owner, 2026-11-04), as proposed. Refines decision 0448's
bounded cache (f3f9552ac, accepted by the owner 2026-11-03). Implemented in
std/native/llrt/heapos.xo (take's request ring, keep's `wanted`).

## Context

Decision 0448 bounded heapos.xo's large block cache: a freed mapping is
kept only while the cache holds under a quarter of the bytes the program
has mapped (membudget.mapped() >> 2). It took coll putget's darwin max RSS
from 8.2 to 6.3 MB and push's by a third, and xo's builds stayed within
noise. The checks before it ran on arm64 only.

The owner's v0.36.0 and v0.38.1 evaluations on the Intel Mac found the
13-perf micro benchmark's json10kx20 slower: 111 ms (0.35.0), 135 (0.36.0),
176 (0.38.1), and peak RSS 59 to 46 MB. Measured by Xo Lang 1 (7
interleaved rounds, best, release builds of each version):

| json10kx20 (ms) | 0.35.0 | 0.36.0 | 0.38.1 |
|---|---|---|---|
| darwin/amd64 (i7-9750H) | 115 | 155 | 177 |
| linux/amd64 (Ryzen 9 9955HX) | 46 | 47 | 82 |
| darwin/arm64 (the Mac) | 47 | 54 | 53 |

A bisect on main between the two releases' fork points (5e6275384,
eda09dfa0; xo built at each commit on the linux/amd64 host) names
f3f9552ac: against its parent, json 50 to 84 ms, sort 64 to 69, map 76 to
84. Each json round frees its encode buffer and decoded lists and asks for
the same sizes again; under the bound the program holds little, so the
cache keeps almost none of them and every round maps and faults them in
afresh (likely costlier with x86's 4 KiB pages than arm64's 16 KiB ones; not measured).

## Decision

On amd64 only (linux, darwin, windows): the cache table keeps a ring of
the last 16 sizes a large allocation asked it for (take, hit or miss), and
keep may pass the quarter bound (never CACHE_N or CACHE_MAX) for a mapping
one of them could use, by take's own fit (need <= size <= 4 need). A
program that frees and asks again for one size keeps those mappings; a
list that grows (each request larger than any block it frees) still gets
the bound, as do one-off large frees. arm64 keeps 0448's rule unchanged:
no variant was faster there, and each raised its RSS as on amd64.

The trade the owner decides: amd64 json back to 0.35.0's time against
the micro's max RSS +6 MB (linux) and +9 MB (darwin), most of the 22% the
v0.38.1 evaluation saw it fall (59 to 46 MB). Every variant that brings
the speed back keeps json's buffers, so it pays that memory.

## Variants measured

linux/amd64 (main at 41314efd6 with each heapos.xo variant; micro best of
7 interleaved; bench/perf/xo putget and push min of 3 runs):

| variant | json | map | sort | strbuild | micro max RSS | putget max RSS | push max RSS |
|---|---|---|---|---|---|---|---|
| main (quarter bound) | 83 | 76 | 69 | 8 | 52.4 MB | 5.06 MB | 40.0 MB |
| no bound | 47 | 65 | 63 | 6 | 58.7 | 5.83 | 38.8 |
| quarter, at least 16 MiB | 49 | 79 | 67 | 8 | 61.6 | 5.83 | 38.8 |
| quarter, at least 4 MiB | 80 | 80 | 69 | 8 | 52.2 | | |
| half | 74 | 73 | 68 | 9 | 58.3 | | |
| reuse credit (any hit lets 4 keeps pass) | 47 | 67 | 64 | 6 | 58.1 | 5.83 | 38.8 |
| want (this decision) | 47 | 65 | 63 | 6 | 58.7 | 5.83 | 29.6 |

Rerun with the 16 size ring (linux/amd64, same method): want16 json 47,
map 66, sort 63, strbuild 6; micro 58.7 MB, putget 5.83, push 29.6.

darwin/amd64, the Intel Mac (i7-9750H; main at origin/main 2026-11-04
with each variant, built on the arm64 Mac; micro best of 7 interleaved,
max RSS min of 3; putget and push best wall and min max RSS of 5):

| variant | json | map | sort | micro max RSS | putget max RSS | push max RSS |
|---|---|---|---|---|---|---|
| main (quarter bound) | 154 | 156 | 88 | 46.4 MB | 4.4 MB | 29.5 MB |
| no bound | 125 | 149 | 93 | 55.5 | 6.3 | 38.8 |
| reuse credit | 127 | 158 | 92 | | 4.6 | 47.5 |
| want, 4 sizes | 136 | 150 | 93 | 55.5 | 5.6 | 27.6 |
| want, 16 sizes (this decision) | 124 | 149 | 93 | 55.5 | 6.3 | 30.0 |

darwin/arm64, the Mac:

| variant | json | map | sort | micro max RSS | putget max RSS | push max RSS |
|---|---|---|---|---|---|---|
| main (quarter bound) | 55 | 51 | 58 | 47.7 MB | 5.6 MB | 30.6 MB |
| no bound | 55 | 52 | 58 | 57.0 | 7.5 | 40.1 |
| want, 4 sizes | 54 | 50 | 59 | 57.0 | 6.8 | 28.8 |
| want, 16 sizes | 55 | 51 | 58 | 57.0 | 7.5 | 31.2 |

darwin/amd64's json is still above 0.35.0's 115 ms with any rule (124 at
best): its 0.35.0 to 0.36.0 step (115 to 155, before f3f9552ac) is a
separate cause, bisected next.

Reuse credit was dropped: on darwin push max RSS rose 31 to 50 MB (a
credit from an unrelated early hit let list growth's frees stay).

## Consequences

- amd64 json10kx20: linux 84 to 47 ms (0.35.0: 46), darwin 154 to 124
  (0.35.0: 115; the rest is the separate 0.36.0 step); map and sort
  better on linux (79 to 66, 69 to 63).
- amd64 max RSS: the micro +6 MB (linux) and +9 MB (darwin); putget +0.8
  and +1.9 MB; push 40.0 to 29.6 MB (linux), 29.5 to 30.0 (darwin).
- The implementation (dee13a3b4) against its base (3f0a4f43c), xo built
  at each on the linux/amd64 host, two alternating runs: json 84 to 47 and
  48 ms, map 84 to 71, sort 69 to 62, strbuild 9 to 6. amd64 sample 51 ok;
  internal/arm64 heap and json goldens pass on the Mac.
- arm64: unchanged (the ring is written, never read there).
- The release check measured arm64 only, which is how this shipped in
  0.37.0 and 0.38.x; the lead adds an amd64 json row to it.

## Alternatives

- Keep 0448's bound: x86 json stays 1.75 to 1.8 times 0.35.0's time.
- No bound, or a 16 MiB floor: the speed back, and 0448's RSS case undone.
- A 4 MiB floor or half of mapped: too little of the speed back.
- Reuse credit: darwin push max RSS +60%.
