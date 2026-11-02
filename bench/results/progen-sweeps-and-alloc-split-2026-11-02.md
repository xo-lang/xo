# Progen differential sweeps and the /alloc commit split (Lang 3, 2026-11-02)

Two sets of results that the 0.36 notes and the blog article cite. Figures are copied from the job logs named
under each heading (kept in the author's session scratch, summarized here); where a count is of a run still in
progress it says "as of" and the final totals replace it.

## 1. Progen differential sweeps

`TestCollDump` (internal/progen, `XO_COLL_MODE` coll, alias, aliasbig or aliasempty; `XO_COLL_FIRST`,
`XO_COLL_COUNT`) writes programs; `scripts/coll-diff.sh` builds each four ways (Go, LLVM, native, native with
every pass off, LLVM the reference) and runs them. A native-only difference is a bug in a pass (the DIFF line
has no `llvm`). Programs whose Go run timed out under load are skipped, not compared (`REFFAIL`).

### linux/arm64 (second offload host, container with 18 CPUs)

Base 1, before f58b4116f (a base at 4e52adafa, with take_in_place; the swapped-lane fix was not in it):

| Family | Seeds | Programs | Same | Differ |
|---|---|---|---|---|
| Alias | 101-800 | 800 | 798 | 2 |
| AliasBig | 1-700 | 600 | 600 | 0 |
| Coll | 10000-10599 | 600 | 600 | 0 |

The two Alias differences are seeds 387 and 666: both are the swapped-lane `slp_f64` pack (a build failure,
"reads virtual register N before it is written") that f58b4116f fixes (golden `slp_build_swapped`). Seed 666
was reproduced on this base, and does not reproduce on a Mac xo built at 1ef022848 or later. One AliasBig batch
(seeds 501-600) had 2 programs whose Go run did not finish under load and were not compared (597, 598).

Base 2, current main (f58b4116f and later), counting run as of 13:19 local time:

| Family | Seeds | Programs | Same | Differ |
|---|---|---|---|---|
| Alias | 601-1500 | 900 | 900 | 0 |
| AliasBig | 701-1500 | 800 | 800 | 0 |
| AliasEmpty | 1-800 | 800 | 800 | 0 |
| Coll | 10600-11399 | 800 | 800 | 0 |

### linux/amd64 (the owner's amd64 host, no LLVM, `SKIP_LLVM`)

Coll 5,000 (seeds 3000-7999) and 1,000 (20000-20999), Alias 3,000, AliasBig 300 and 1,000, AliasEmpty 200,
1,000 (2000-2999): all same. The amd64 backend has no `slp_f64`, so Alias on amd64 cannot reach the arm64 class.

### Earlier arm64 results

Coll 5,400 programs same before the counting driver; goldens quarters 1 to 3 on arm64: only Go backend
differences (`list_swap_guard`, `native_sql`, `map_log_model`, `fair_pairs`, `use_c_add`, `use_c_enum`),
handled in a489bb59c (`want_go.txt`, Go timeouts skipped).

### Bugs the sweeps found

thread_cset (469c7f82d, found by Coll: 36 of 40 programs on the buggy seed), slp_f64 (4f3a667a4 and 51bb3a596,
found by Alias: 3 of 100 on a blind first run, later seeds 118, 162, 173, 177, 178, 189, 239, 295), the
swapped-lane shape (seed 387, f58b4116f), take_in_place (4e52adafa, AliasBig), the separate-compilation link
failure of byref_struct and byref_held (72ee6bd1e; a warm shared backend cache hides separate compilation).

## 2. /alloc split by commit (job l3pts)

bench/perf/jitter's `/alloc` (200 items, about 1,000 fields, built and JSON encoded per request), on the first
linux/arm64 host, exclusive, 3 rounds, medians, servers on CPUs 0-5 and the load generator on 6-11 with the
runner `alloc-points-remote.sh` (Lang 4). Ratios to v0.35.0's server built in the same job (base: 164,753 req/s
at 32 connections, 173,395 at 256).

| Point | c32 req/s | c32 CPU/req | c256 req/s | c256 CPU/req |
|---|---|---|---|---|
| f58b4116f | 0.881 | 1.157 | 0.732 | 1.367 |
| 26a0ccd33^ (d1629f060) | 0.882 | 1.153 | 0.740 | 1.351 |
| d71e1b442^ (132d3934f) | 0.873 | 1.167 | 0.735 | 1.360 |
| d71e1b442 | 0.776 | 1.322 | 0.658 | 1.518 |
| a391cef12^ (1d24c4b9d) | 0.775 | 1.322 | 0.652 | 1.534 |
| a391cef12 | 0.781 | 1.314 | 0.659 | 1.516 |
| 1ef022848 | 0.777 | 1.320 | 0.655 | 1.527 |

The step is at d71e1b442 (one more field in `Response`); the heap profile and THP commits are flat. The park
rule amplified it (see the 20 ms linger, 17ded84bb).

Exact allocation counts of the handler's work (heap profile, rate 1, 40 iterations of 200 items plus the JSON
encode, darwin/arm64): 820 objects per request at eff43904e and at main; bytes per request 88 KB against 95 KB
(+8%), all in `page` (the list floor capacity of 4 slots, 200 `tags` lists x 32 bytes), so the list growth
commits (8fe1f704b, 6956acf48, 8d36947ae) add bytes but no allocations or copies.

## 3. Moving builds out of the timing lease

`alloc-split.sh` (Lang 3, `~/xo-lead/notes/xo-lang-4/scripts`) builds on the second host and leaves only the
measuring in host 1's exclusive lease. A two revision test: the build step took 33 s of work (565 s wall,
waiting for the host at load 30). Minutes saved per job: see the follow-up line in this file when measured.
