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

## Stage 2: the objects stay C (measured; they miss the bar on arm64)

Two Xo versions of the objects were built and measured; neither met
decision 0180's bar on arm64, so the string and list objects, the drop
worklist, the real-time sections, the builders, and const_once stay C.
The Xo code is in commit 3da29c8b (std/native/llrt/obj.xo, cabi.xo's
`obj_*` exports, xo_rt.c forwarders with a heap word: the thread's heap
record, plus 1 when other threads run), reverted after the measurement.

### Version 1: strings and lists (Mac, `/usr/bin/time -l`)

obj.xo held rs_new, rs_append (the body of xo_rs_concat and every C
builder), xo_rl_new, rl_copy, rl_share, rl_grow, rl_free, and
xo_rc_defer's worklist. Std A at 5733ae10, std B the change, min of 5:

| Workload | arm64 | LLVM |
|---|---|---|
| alloc small | 1.326 | 1.158 |
| alloc small (multi threaded paths out of line) | 1.224 | 1.071 |
| alloc strings (same) | 1.076 | 0.955 |
| alloc maps (same) | 1.053 | 1.012 |
| text split (same) | 1.094 | 1.029 |
| text lines (same) | 1.077 | 1.025 |
| text int_fmt (same) | 1.064 | 1.037 |
| text debug (same) | 1.058 | 0.949 |
| json enc_small | 1.055 | 0.924 |
| nested walk | 1.068 | 1.009 |
| xo, 18 workloads | 0.999 to 1.009 | 0.991 to 1.001 |

### Version 2: lists and the worklist only (linux/arm64, EC2)

The string objects back in C; obj.xo with xo_rl_new, rl_copy,
rl_share, rl_grow, rl_free's element loop (a list without a drop
freed by the C directly), and xo_rc_defer's worklist. The Mac was out of
disk (7 GB free, its local runs paused by the lead), so this ran on a
warm r8g (i-EXAMPLE06, commit 3da29c8b, clang 15.0.7):
`perf stat -e instructions:u`, min of 5, the same A and B std trees.

| Workload | arm64 | LLVM |
|---|---|---|
| alloc small | 1.266 | 1.093 |
| alloc strings | 1.045 | 1.010 |
| alloc maps | 1.037 | 0.994 |
| alloc mixed, grow | 1.002 to 1.020 | 0.994 to 0.998 |
| nested walk | 1.115 | 1.023 |
| nested shared | 1.017 | 0.992 |
| nested, the other 6 | 1.000 to 1.005 | 0.993 to 1.013 |
| xo, 18 workloads | 1.000 to 1.005 | 1.000 to 1.001 |
| text split | 1.098 | 0.998 |
| text lines | 1.069 | 0.993 |
| text int_fmt | 1.053 | 0.999 |
| text quote, replace, float_fmt, debug | 1.004 to 1.027 | 0.986 to 1.011 |
| json, 6 workloads | 1.003 to 1.029 | 0.996 to 1.018 |

The self build (xo built from std A and from std B, each compiling
selfhost/xo with the same std, `XO_LEAKCHECK=1`, two runs, perf counts
the cc children too): 1,404,959,722,773 and 1,405,012,040,955
instructions (A), 1,420,893,176,316 and 1,420,803,199,718 (B), 1.011;
both 0 live of about 1,096 million allocations; the two outputs
byte identical.

The same commit passed (so the Xo version is correct, only slower):
`xo dev goldens` 181 of 181 on arm64 and on LLVM, internal/llvm
TestRuntime*, TestCABIDiff, TestLeakCheck, TestHTTP1*, and the fixed
point (scripts/bootstrap-xo.sh: s2 and s3 assembly and binaries
identical, 52,292,040 bytes; i-EXAMPLE01). internal/arm64
TestGoldenAsm differed (its fib and sieve goldens hold the program's
cabi exports, which obj.xo added to).

### Why arm64 misses it (read in `xo build --asm`)

- No shrink wrapping: an arm64 function saves every callee saved
  register it uses in its prologue, on every path. rl_free's loop over
  the drops saved five register pairs on the path with no drop (hence
  the C fast path in version 2), rs_append's in place path four pairs
  that only its copy path needs, and the first version's single
  threaded allocation five pairs for an inlined alloc.refill
  (version 1's 1.326, out of line in the second row: a never taken
  self call keeps a function out, since selfhost/arm64/inline.xo does
  not inline a function on a call cycle).
- `rt.mem_copy` and `rt.mem_move` are calls of memcpy and memmove;
  clang stores a constant one byte append (xo_rs_push_byte) inline.
- Each level costs a frame record: generated code to the C entry (kept:
  generated code and seed 0.10.0-23ce8ce2 call xo_rl_new and the others
  by name), the export's branch, the export, then alloc.xo. clang
  inlines the C's levels; LLVM's LTO inlines the Xo ones, which is why
  LLVM sits near 1.0 while arm64 does not.

### Next experiments (ranked by likelihood)

1. Shrink wrapping in selfhost/arm64 (save callee saved registers only
   on the paths that use them), then rerun version 2: check out
   3da29c8b's obj.xo, cabi.xo, xo_rt.c and run this file's A/B on
   bench/perf/alloc small, nested walk, text split and lines.
2. Inline `rt.mem_copy` and `rt.mem_move` for a length known small
   (one to sixteen bytes: loads and stores) in both arm64 emitters,
   then version 1's string objects.
3. The export symbol as an alias of the Xo function instead of a
   branch (raw-memory-2026-10-13.md's own next step), and generated
   code calling `xo_x_obj_*` directly once a seed has them (the C
   entry points then go with the next seed).
4. Port only the cold list paths (rl_copy, rl_share, rl_grow: 0.992 to
   1.017 on their workloads in version 2) once the hot ones can follow,
   so the list code is not in both languages meanwhile.
