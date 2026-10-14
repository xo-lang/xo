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

## Stage 2: the objects

### The string objects stay C (measured, did not meet the bar)

std/native/llrt/obj.xo first held rs_new and rs_append too (the C's
rs_new, rs_append, xo_rs_concat's body), called from xo_rt.c through
cabi.xo exports with a heap word. Same method (std A at 5733ae10, std B
the change), bench/perf/alloc, text, json, nested, xo:

| Workload | arm64 | LLVM |
|---|---|---|
| alloc small (first version) | 1.326 | 1.158 |
| alloc small (mt paths out of line) | 1.224 | 1.071 |
| alloc strings | 1.076 | 0.955 |
| alloc maps | 1.053 | 1.012 |
| text split | 1.094 | 1.029 |
| text lines | 1.077 | 1.025 |
| text int_fmt | 1.064 | 1.037 |
| text debug | 1.058 | 0.949 |
| json enc_small (first version) | 1.055 | 0.924 |
| nested walk (first version) | 1.068 | 1.009 |
| xo, 18 workloads (first version) | 0.999 to 1.009 | 0.991 to 1.001 |

Causes, read in the arm64 assembly (`xo build --asm`): the arm64
emitters save every callee saved register a function uses in its
prologue, on every path (no shrink wrapping), so rs_append's in place
path saves four register pairs that only its copy path needs;
`rt.mem_move` and `rt.mem_copy` are calls of memmove and memcpy where
clang stores a one byte append (xo_rs_push_byte) inline; and each Xo
level adds a frame record that the C, inlined by clang, does not have.
In the first version obj.xo's single threaded allocation also carried an
inlined alloc.refill (five register pairs saved per call); obj.xo now
keeps the multi threaded paths out of line (a never taken self call,
since the arm64 inliner skips functions on a call cycle).
