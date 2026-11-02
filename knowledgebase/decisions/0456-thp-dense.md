# 0456: XO_THP=dense, huge pages for heap chunks a thread fills

Status: Accepted (owner, 2026-11-02, with v4 as Linux's default; proposed by Xo Lang 2,
2026-11-02).

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

XO_THP=dense, v4, is Linux's default (the owner chose v4 on 2026-11-02). XO_THP=off keeps
decision 0454's off, and XO_THP=kernel gives no advice at all (the default before this
decision). Darwin, Windows and wasm are unchanged.

- Bump and page chunks are mapped MADV_NOHUGEPAGE, as with off.
- A heap record's first chunk opens its first 2 MiB half only. When the bump uses that half
  up, the second half is advised MADV_HUGEPAGE and opened (heapos.reserve with decision
  0455's limit word; no mprotect, the half stays accessible).
- The main thread's record is the exception: its first chunk is advised MADV_HUGEPAGE whole,
  since a program's single threaded work (a build's front end) allocates there densely from
  the start.
- Once a heap record has used a chunk up, that chunk (for khugepaged) and the record's next
  chunk are advised MADV_HUGEPAGE. The advice comes at map time, before the next chunk's pages
  fault, so a busy record's later chunks get huge pages at once.
- Large blocks keep the kernel's policy.

Every check is on the chunk refill path (heapos.chunk, pchunk, reserve, extend): no
allocation fast path changes. Code: std/native/llrt/heapos.xo (thp_mode, dense_halves,
thp_filled), entry.xo (XO_THP).

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

v4 (the decision), against 2d90b65d9, linux/arm64 always, 5 rounds: coll edit wall 1.017
(1.01-1.02), CPU 1.013, max RSS 0.866; self wall 1.010 (1.006-1.013), CPU 0.998, max RSS
0.998. linux/amd64 madvise: coll edit wall 0.928 (0.916-0.947), CPU 0.942, max RSS 1.145.
2,000 held TLS connections, v4's build (WSL, THP set to `always` for the run and restored),
RssAnon MB over 3 rounds: XO_THP=kernel 125-138, v4 63-64, Go 87-95
(bench/results/thp-dense-2026-11-02/tls-v4-wsl.txt). The table above is v2, the simpler form
first proposed. The confirming run of the landed
change is in the CHANGELOG entry.

## Consequences

- THP `always` (most cloud images): an idle server's worker threads stay on small pages
  (their first chunk's second half is never opened), without off's cost to the threads that
  allocate densely. With v4 the self build pays 1% wall, and coll's edit 1.7% wall for 13%
  less max RSS.
- THP `madvise` (Ubuntu's default, WSL): MADV_HUGEPAGE is new there, so threads that fill
  chunks now get huge pages: coll's edit 7% faster for 14.5% more max RSS.
- The main thread's first chunk takes huge pages from the start, so a server's main thread
  costs up to one 4 MiB chunk of huge pages more than off.
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
- v2, a record's first chunk wholly NOHUGEPAGE and its next chunks HUGEPAGE (the table
  above): coll edit 1.043 on always, where v4 pays 1.017; proposed first, not taken.
- Go's shared page heap: the full fix for density, a large change (next ADR).
