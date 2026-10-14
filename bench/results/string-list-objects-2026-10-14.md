# String and list objects (zero-C audit B13), 2026-10-14

Work item B13 of `bench/results/zero-c-audit-refresh-2026-10-13.md`
(decision 0195). Bar: decision 0180 (instructions of the Xo path over
the C path's, on the Mac, `/usr/bin/time -l`, minimum of 5 runs, less
the start and stop alone where the workload has a `none` row). Ten
agents share the machine; instruction counts are stable to about 1
percent, the HTTP client's to about 5 (network waits, see run 1).

Method: one xo (this branch's, built by seed 0.10.0-23ce8ce2 in 57 s)
and two std trees through `XO_STD`: A, the branch base (e4ca29d8), and
B, the change. The compiler change of stage 1 (show.xo) does not touch
these programs.

## Stage 1: two reroutes

- std/native/llrt/http1.xo: every `rt.str_find` is `scan.find`
  (decision 0201's SIMD scan), and the single byte searches in a range
  (`find(s, c, from, to)`, the line ends) are `scan.find_byte` bounded
  by the range, where rt.str_find searched on to the end of the string.
  xo_std.c's `xo_str_find` and `n_mem_index` (36 lines) are deleted;
  `rt.str_find` stays declared for the wasm runtime, which has its own.
- selfhost/ssa/show.xo: the stack trace's padding counts runes with the
  native prelude's `str_rune_count` (scan.rune_count) instead of
  `rt.str_rune_count`. xo_std.c's `xo_str_rune_count` stays as a labeled
  seed shim: seed 0.10.0-23ce8ce2 lowers the padding to it
  (gotchas/runtime-c-the-seed-still-calls.md); delete it with a seed
  that has B13.

### bench/perf/httpparse (server, N=20000)

| Workload | arm64 | LLVM |
|---|---|---|
| h1_get | 0.990 | 1.012 |
| h1_chunked | 0.984 | 1.013 |
| h1_url | 0.961 | 0.968 |

With scan.find alone (before the bounded find_byte): h1_get 1.004 and
1.029, h1_chunked 1.001 and 1.037, h1_url 1.025 and 1.005.

### bench/perf/httpclient (client, N=3000), two runs

| Workload | arm64 run 1 | LLVM run 1 | arm64 run 2 | LLVM run 2 |
|---|---|---|---|---|
| len | 0.983 | 0.996 | 1.017 | 1.012 |
| chunked | 1.051 | 1.076 | 0.994 | 0.998 |
| big | 0.991 | 1.023 | 1.002 | 1.021 |
| redirect | 1.082 | 0.995 | 1.019 | 1.010 |

Run 1's 1.051 to 1.082 did not repeat (run 2, the same binaries'
sources, at most 1.021); the client waits on a local server, and its
counts move by several percent between runs of the same build.
