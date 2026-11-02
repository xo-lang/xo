# 0456: XO_THP=dense, huge pages for heap chunks a thread fills

Status: Proposed (Xo Lang 2, 2026-11-02; needs the owner's acceptance, allocator change:
whether dense becomes Linux's default is the owner's decision).

## Context

Decision 0454 added XO_THP=off. Under THP `always`, Xo's per thread 4 MiB bump and page
chunks are 13-23% filled, and each first touch of a 2 MiB half faults a whole huge page, so a
sparse heap's RSS grows 2 to 3 times: 2,000 held TLS connections take 95-108 MB, and 40-56 MB
with off. Off costs the self build 7.5% CPU, so the default stayed.
bench/results/thp-density-2026-11-02.md found why Go stays low under `always` without turning
huge pages off: its single address ordered page heap fills the 2 MiB regions it touches
(71-89%). bench/results/thp-dense-2026-11-02.md tried the cheap half of that rule on Xo's
chunks: only a thread that has filled a chunk allocates densely, so only its chunks should
take huge pages.

## Decision

XO_THP=dense (Linux; the variable is read at start, like off; no default change in this
ADR):

- Bump and page chunks are mapped MADV_NOHUGEPAGE, as with off.
- Once a heap record has used a chunk up, that chunk (for khugepaged) and the record's next
  chunk are advised MADV_HUGEPAGE. The advice comes at map time, before the next chunk's pages
  fault, so a busy record's later chunks get huge pages at once and its first never does.
- Large blocks keep the kernel's default.

Every check is on the chunk refill path (heapos.chunk, pchunk): no allocation fast path
changes. The prototype is branch l2-dense2 (c8fe48019, on 0b77e3048), unpushed.

## Numbers

Dense as the default (a measure only commit) against its base, `xo bench --ab` through
ab-host.sh; self is the self build, coll edit the warm rebuild of bench/perf/xo, alloc
bench/perf/alloc. Logs: bench/results/thp-dense-2026-11-02/.

| Host, THP | self wall | coll edit wall / max RSS | alloc wall / max RSS |
|---|---|---|---|
| linux/arm64, always | 0.997 (0.995-1.008, 5 rounds), RSS 0.997 | 1.043 (1.004-1.049) / 0.79 | 0.992 / 0.71 |
| linux/amd64 (WSL), madvise | 0.944 (0.935-0.962, 5 rounds), RSS 1.015 | 0.952 (0.935-0.971) / 1.074 | 0.984 (noise) / 1.0 |

2,000 held TLS connections, RssAnon MB, 3 rounds (WSL with THP set to `always` for the run,
with the owner's approval): Xo default 109-121, off 56-65, dense 50-64, Go 86-88. No idle
record ever fills a chunk (about 260 KiB used each), so for an idle server dense is off.

## Consequences

- THP `always` (most cloud images): a server's memory is off's, without off's cost to the
  threads that allocate densely: the self build is unchanged. A short build pays: coll's edit
  4% wall for 21% less RSS.
- THP `madvise` (Ubuntu's default, WSL): MADV_HUGEPAGE is new there, so threads that fill
  chunks now get huge pages: the self build 5.6% faster, coll's edit 4.8% faster, for 1.5% to
  7% more RSS.
- A few madvise calls per heap record over its life.
- Go's other half, one address ordered page heap whose touched regions are full, is what keeps
  Go both fast and low. That is the larger change, decided separately.

## Alternatives

- Off by default: self build CPU +7.5% (decision 0454).
- v1, advising a chunk only after it is used up: costs what off costs (self 1.060, coll edit
  1.097), because the pages have already faulted small.
- v3, a record's first chunk opened by 2 MiB halves, the second advised HUGEPAGE when the first
  fills (51ba0c8df): coll edit 1.034, RSS 0.813; self 1.013, CPU 1.0. Pages first faulted
  under NOHUGEPAGE stay small until khugepaged, so halves only shave the cost.
- v4, as v3, but the main thread's record keeps its first chunk HUGEPAGE (9da48c6b6): on
  always, coll edit 1.017 (1.01-1.02), RSS 0.866, self 1.010 (CPU 0.998); on madvise, coll
  edit 0.928, RSS 1.145. It halves coll's cost for a few MB on servers; not taken here
  (owner direction 2026-11-02: decide the simple form).
- Go's shared page heap: the full fix for density, a large change (next ADR).
