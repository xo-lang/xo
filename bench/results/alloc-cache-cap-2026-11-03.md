# Allocator cache cap: coll peak memory, 0.35.0 to 0.37.0 (2026-11-03)

Peak memory (maximum resident set size) of bench/perf/xo `coll putget`
(3 million puts over 50,000 map keys, then reads) and `coll push`
(3 million integer appends, then a sum) at each step of the 0.36.0
memory rise and its fix, and of the same workloads in Go
(bench/perf/go), for the allocator post on the website. Raw output is in
[alloc-cache-cap-2026-11-03/](alloc-cache-cap-2026-11-03/); the
decision is [0448](../../knowledgebase/decisions/0448-compiler-memory.md)
(the capped large-block cache) and
[0469](../../knowledgebase/decisions/0469-thp-dense-no-main-exception.md) (no huge
page advice for the main thread's first chunk).

## Builds

| Label | Commit | What |
|---|---|---|
| 0.35.0 | 216203685 | tag v0.35.0 |
| 0.36.0 | 5942fb280 | tag v0.36.0 |
| + 4n | e68bf2fac | lists and strings past the size classes reserve 4 times their size |
| pre-cap | b828067a8 | the parent of f3f9552ac |
| + cap | f3f9552ac | large-block cache capped at 1/4 of bytes in use (decision 0448) |
| main pre-cap (linux THP runs) | 9296855bc | main with + 4n, before the cap |
| cap branch (linux THP runs) | 5d8b6faac | the cap before its rebase (same allocator as f3f9552ac) |
| + 0469 | b376345b6 | the main record's first chunk not advised HUGEPAGE (pushed as 3389eb2c0) |

## Method

Each build's `coll` binary was built by `xo build` from a checkout of
that commit (its own std), run N times per workload, and the median max
RSS reported. The scripts lived in a scratch directory (repro scripts
are not kept in the repository); they ran:

- macOS: `/usr/bin/time -l` (bytes, divided by 1024), 7 runs, Go
  1.26.1 `go build` of bench/perf/go. The wall column has 10 ms
  resolution and is not a timing result; timing claims come from the
  `--ab` runs in decision 0448.
- linux/arm64 (offload host 1): GNU `/usr/bin/time -f "%M %e"` (KB).
  The host runs transparent huge pages enabled=always, defrag=madvise
  (first line of each linux file), so the THP runs set `XO_THP` to
  each mode (decision 0454): `kernel` (no advice), `dense` (the
  default), `off`. 5 runs each.

## macOS (darwin/arm64), 7 runs

From [darwin.txt](alloc-cache-cap-2026-11-03/darwin.txt) (Darwin 27.0.0,
12 CPUs, Go 1.26.1), max RSS KB, median:

| Build | putget | vs 0.35.0 | push | vs 0.35.0 | darwin.txt |
|---|---|---|---|---|---|
| 0.35.0 | 7,296 | 1.000 | 50,944 | 1.000 | lines 2, 3 |
| 0.36.0 | 8,624 | 1.182 | 56,096 | 1.101 | lines 4, 5 |
| + 4n | 8,288 | 1.136 | 41,568 | 0.816 | lines 6, 7 |
| pre-cap | 8,256 | 1.132 | 41,584 | 0.816 | lines 8, 9 |
| + cap | 6,304 | 0.864 | 26,832 | 0.527 | lines 10, 11 |
| Go 1.26.1 | 8,784 | 1.204 | 57,936 | 1.137 | lines 12, 13 |

## linux/arm64, putget by XO_THP mode, 5 runs

From [linux-thp-2.txt](alloc-cache-cap-2026-11-03/linux-thp-2.txt) and
[linux-thp-3.txt](alloc-cache-cap-2026-11-03/linux-thp-3.txt), max RSS
KB, median:

| Build | kernel | dense (default) | off | lines |
|---|---|---|---|---|
| 0.35.0 | 10,352 | 10,352 | 10,356 | thp-2 2 to 4 |
| 0.36.0 | 17,028 | 12,964 | 7,860 | thp-2 5 to 7 |
| main pre-cap | 15,284 | 12,644 | 6,452 | thp-2 8 to 10 |
| cap branch | 13,344 | 10,704 | 5,604 | thp-2 11 to 13 |
| + cap (f3f9552ac) | 13,344 | 10,704 | 5,604 | thp-3 2 to 4 |
| + 0469 | 13,344 | 8,696 | 5,604 | thp-3 5 to 7 |

0.35.0 predates the huge page modes, so its three rows are the same
program. With huge pages off, the capped build peaks at 0.54 of 0.35.0
(5,604 against 10,356); in the default dense mode the main thread's
first 4 MiB chunk was advised HUGEPAGE from the start, which decision
0469 removed: dense then peaks at 8,696 KB, 0.84 of 0.35.0.

## linux/arm64 census, 7 runs

From [linux.txt](alloc-cache-cap-2026-11-03/linux.txt) (host 1,
12 CPUs, THP enabled=always, Go 1.26.1), default `XO_THP` (dense), max
RSS KB, median:

| Build | putget | vs 0.35.0 | push | vs 0.35.0 | linux.txt |
|---|---|---|---|---|---|
| 0.35.0 | 10,352 | 1.000 | 54,780 | 1.000 | lines 3, 4 |
| 0.36.0 | 14,348 | 1.386 | 58,108 | 1.061 | lines 6, 7 |
| + 4n | 12,712 | 1.228 | 44,332 | 0.809 | lines 9, 10 |
| pre-cap | 12,644 | 1.221 | 44,328 | 0.809 | lines 12, 13 |
| + cap | 10,684 | 1.032 | 33,752 | 0.616 | lines 15, 16 |
| + 0469 (3389eb2c0) | 8,696 | 0.840 | 31,764 | 0.580 | linux-0469.txt lines 3, 4 |
| Go 1.26.1 | 9,988 | 0.965 | 74,276 | 1.356 | lines 17, 18 |

The 0469 row ran in a second job on the same host
([linux-0469.txt](alloc-cache-cap-2026-11-03/linux-0469.txt)), whose
Go runs measured putget 9,996 KB and push 88,232 KB (lines 5, 6).

0.36.0's putget runs are bimodal (12,900 or about 14,400 KB, line 6);
the release run measured 13,634 KB and the 5-run THP table above
12,964 KB, so its median here depends on the split. Go's push runs
range from 66,236 to 88,380 KB (line 18).

## 0.37.0 as released, 7 runs

The 0.37.0 seed build, 849e98465 (both the cap and decision 0469),
measured the same way, with Go again in the same job:

| Host | putget | vs 0.35.0 | push | vs 0.35.0 | Go putget | Go push | file |
|---|---|---|---|---|---|---|---|
| darwin/arm64 | 6,288 | 0.862 | 26,784 | 0.526 | 8,864 | 58,016 | [darwin-0.37.0.txt](alloc-cache-cap-2026-11-03/darwin-0.37.0.txt) lines 2 to 5 |
| linux/arm64 (dense) | 8,772 | 0.847 | 31,840 | 0.581 | 9,996 | 80,380 | [linux-0.37.0.txt](alloc-cache-cap-2026-11-03/linux-0.37.0.txt) lines 3 to 6 |

The ratios are against the 0.35.0 rows above (darwin.txt line 2 and 3,
linux.txt line 3 and 4). Linux putget is 76 KB over 3389eb2c0's 8,696;
Go's push again spreads widest (55,992 to 103,240 KB).
