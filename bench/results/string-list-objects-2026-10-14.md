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

## Stage 2, first attempt: the objects stay C (measured; they miss the bar on arm64)

Superseded by "Stage 2 re-landed" below (decision 0226): the list
objects and the drop worklist are in Xo; the string objects stay C.

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

## Stage 2 re-landed: the lists without a C level (decision 0226)

Branch b13-stage2: 3da29c8b's obj.xo, cabi.xo and xo_rt.c re-applied on
main 97472682, then the C level removed from the allocation path. arm64
generated code calls obj.xo's entries (`xo_x_obj_rl_new_n`, `_grow_n`,
`_free_n`, `_copy_n`, `xo_x_obj_rc_defer_n`, `xo_x_obj_rl_share`)
directly; they find the heap in process words 10 to 13 and the thread
local table, and their common cases (one thread, no real-time section,
a small block from the free list or the chunk: `alloc.alloc_fast`
inlined) make no call and so have no frame. The C entries forward to the
same exports for LLVM and the seed. The string objects stay C.

Method: the Mac (12 cores, the heavy lock, other sessions running),
`/usr/bin/time -l` instructions retired, minimum of 5 runs, less the
`none` row where the program has one (alloc, text, json). A is main
97472682 (its xo and std: the C list objects); B is this branch (its xo,
built by itself from this tree, and its std). Same program sources;
every checksum equal.

| Workload | arm64 | LLVM |
|---|---|---|
| alloc small | 0.927 | 0.966 |
| alloc strings | 1.000 | 1.000 |
| alloc maps | 1.002 | 1.011 |
| alloc mixed | 1.007 | 1.005 |
| alloc grow | 1.001 | 0.999 |
| alloc tasks | 0.998 | 0.883 |
| nested push | 0.998 | 1.002 |
| nested setnested | 1.001 | 1.006 |
| nested setdeep | 0.994 | 1.000 |
| nested mapput | 0.999 | 1.002 |
| nested strappend | 0.997 | 1.004 |
| nested method | 1.007 | 1.005 |
| nested shared | 1.004 | 1.007 |
| nested walk | 0.981 | 0.997 |
| text split | 1.021 | 1.011 |
| text lines | 1.001 | 0.995 |
| text int_fmt | 1.001 | 1.001 |
| text quote | 1.000 | 1.000 |
| text replace | 1.002 | 1.001 |
| text debug | 1.000 | 1.000 |
| text float_fmt | 1.001 | 1.000 |
| text trim | 1.001 | 1.000 |
| json small | 0.998 | 1.000 |
| json ints | 1.000 | 0.997 |
| json strings | 1.000 | 0.999 |
| json maps | 1.002 | 1.005 |
| json parse | 1.000 | 1.000 |
| json enc_small | 1.000 | 0.999 |
| json enc_strings | 1.000 | 0.999 |
| xo, 18 workloads | 0.998 to 1.003 | 0.998 to 1.001 |
The self build: xo A and xo B each build main's selfhost/xo with main's
std (`--std`, `XO_EMIT_JOBS=8`, fresh `XO_BUILD_DIR`, `XO_BIN_CACHE=off`,
`XO_BACKEND_CACHE=off`, `XO_LEAKCHECK=1`; the xo process's own
instructions, not the cc it runs), two runs: A 57,602,443,363 and
57,474,596,555, B 57,580,146,992 and 57,548,699,968 (0.9996 and 1.0013);
both 0 live of 28,270,317 allocations; the outputs byte identical.

### What it took (each step measured on the workloads above)

1. The direct entries and the heap words (rl_new_n first): alloc small
   on arm64 still 1.212, because `[i]` is the empty literal plus a push
   (rl_grow) and its free is xo_rl_free, both still behind the C. With
   rl_grow_n and rl_free_n direct: alloc small 0.927, but nested walk
   1.052 and text split 1.072 (the bump allocation and growth past four
   took heap_word and two more levels).
2. alloc_fast carves from the chunk too, and the single threaded cases
   call rl_new, rl_grow and rl_free with the heap record directly: nested
   walk 0.987, text split 1.051, lines 1.037, alloc strings 1.023.
3. alloc strings has no list code: obj.xo's new calls of alloc.alloc and
   alloc.free made them three sites and more, so the arm64 inliner no
   longer copied them into cabi.heap_alloc (73 instructions to 4) and
   heap_free (17 to 4), every C allocation's path
   (gotchas/arm64-inline-few-sites-new-callers.md). halloc and free1 as
   the only call sites: alloc strings 0.999, text split 1.014, lines
   1.000; alloc tasks 1.065.
4. alloc tasks (the threaded paths): traced with lldb, one iteration was
   262 instructions in A and 280 in B, the excess in rl_grow's prologue
   (five register pairs saved for the resize path the empty literal's
   first push does not take). The resize moved to `grow`, out of line:
   0 instructions per iteration over A on a one task microbenchmark.
5. LLVM, whose generated code still calls the C entries: with those
   forwarding through hw() and the hw exports, alloc small 1.250 and
   tasks 1.103. Forwarding to the `_n` exports (LTO folds the C level):
   0.966 and 0.883. Last, json enc_small 1.051 and text replace 1.028 on
   LLVM: clang inlined xo_resize into rs_append once it was the only
   caller (182 to 402 instructions); `noinline` brings them to 1.000.
6. A bug found by the self build, not the benchmarks: rl_grow_n grew the
   immortal empty list when its fast allocation failed (elements too
   large for the small classes), and the xo built with it crashed at
   once (gotchas/broken-xo-becomes-next-seed.md). Fixed before the
   final run above; internal/llvm/testdata/golden/list_objects covers
   it (it crashes with the bug on arm64).

### Next

- Delete the C entry points (xo_rl_new_n, xo_rl_grow, xo_rl_free,
  xo_rl_copy, xo_rl_share, xo_rc_defer) once a seed emits the `_n`
  calls and LLVM generated code calls the exports (casts at the call
  sites, or the Xo functions by name).
- The string objects (rs_new, rs_append, the builders): the same
  shape, with generated code calling Xo entries directly.

## Stage 3: the string objects (branch b13-strings)

obj.xo makes and grows the string objects: `rs_append` (the body of
`a + b`, xo_rs_concat, xo_rs_append, and the C's rs_append), and the
builders `rs_push_byte`, `rs_push_word`, `rs_push_words`,
`rs_append_bytes` (with the ASCII case fold), and `rs_with_cap`, through
cabi.xo's `obj_rs_*` exports with the C ABI (the string object and
length in, the new length stored through a pointer). arm64 generated
code calls the exports directly: `a + b` and the runtime calls
`rt.rs_append`, `rt.rs_push_byte`, `rt.rs_push_word`,
`rt.rs_push_words`, `rt.rs_append_bytes`, and `rt.rs_with_cap` from
std/native. The C entries forward to them (for LLVM and the seed), and
the C's own rs_append (xo_rs_append_rune, xo_rs_concat_int and _float)
too. Every append goes through `reserve`: a unique string with the room
is returned as it is (no call, no frame: the arm64 code of
xo_x_obj_rs_push_byte's fast path is eleven instructions), the rest is
reserve_slow, out of line (in place growth by doubling, or a copy and
the release of the old one). The C's rs_new stays for the C creators
(xo_str_slice, the number text, standard input): forwarding it to Xo
took text lines to 1.054 and split to 1.049 on LLVM, since clang no
longer inlined the allocation into xo_str_slice
(gotchas/c-helper-forwarded-to-xo-loses-inlining.md).

Method: as stage 2. A is main c0f1de3a (its xo, built by seed 0.11.0,
and its std); B is the branch (its xo, built by the same seed, and its
std). Same program sources, every checksum equal, min of 5 instructions
retired, net of the none row; two rounds:

| Workload | Backend | A round 1 | B round 1 | B / A | A round 2 | B round 2 | B / A |
|---|---|---|---|---|---|---|---|
| text split | arm64 | 96,887,124 | 96,380,907 | 0.998 | 96,822,356 | 96,371,078 | 0.996 |
| text lines | arm64 | 153,977,473 | 153,684,017 | 1.000 | 153,859,979 | 153,657,655 | 1.000 |
| text int_fmt | arm64 | 81,153,711 | 80,017,505 | 0.986 | 81,113,753 | 80,009,904 | 0.984 |
| text quote | arm64 | 144,923,464 | 130,613,220 | 0.884 | 144,850,211 | 130,574,924 | 0.883 |
| text replace | arm64 | 88,808,252 | 82,642,038 | 0.910 | 88,786,758 | 82,631,985 | 0.908 |
| text debug | arm64 | 277,891,705 | 227,139,218 | 0.801 | 277,851,775 | 227,114,810 | 0.801 |
| text float_fmt | arm64 | 148,950,680 | 147,499,655 | 0.991 | 148,968,911 | 147,443,879 | 0.989 |
| text trim | arm64 | 154,961,395 | 154,712,987 | 1.001 | 154,971,966 | 154,647,777 | 0.999 |
| text split | llvm | 79,884,504 | 79,317,796 | 0.996 | 79,828,038 | 79,310,937 | 0.994 |
| text lines | llvm | 130,148,217 | 129,931,661 | 1.001 | 130,093,452 | 129,919,637 | 1.000 |
| text int_fmt | llvm | 65,009,187 | 64,933,970 | 1.005 | 65,010,176 | 64,853,180 | 1.000 |
| text quote | llvm | 119,576,104 | 112,735,943 | 0.934 | 119,526,798 | 112,602,896 | 0.932 |
| text replace | llvm | 67,627,615 | 65,459,063 | 0.961 | 67,560,090 | 65,432,565 | 0.959 |
| text debug | llvm | 235,396,698 | 201,379,872 | 0.843 | 235,391,968 | 201,355,577 | 0.843 |
| text float_fmt | llvm | 108,769,642 | 108,771,626 | 1.004 | 108,747,043 | 108,740,920 | 1.002 |
| text trim | llvm | 143,717,207 | 143,459,783 | 1.000 | 143,666,896 | 143,479,451 | 1.000 |
| json small | arm64 | 348,431,313 | 344,921,749 | 0.997 | 348,407,668 | 344,852,211 | 0.997 |
| json ints | arm64 | 234,570,029 | 234,337,742 | 1.013 | 234,534,003 | 234,271,361 | 1.012 |
| json strings | arm64 | 376,535,432 | 360,913,169 | 0.961 | 376,509,247 | 360,766,439 | 0.961 |
| json maps | arm64 | 335,042,361 | 337,736,708 | 1.018 | 334,963,483 | 337,698,168 | 1.018 |
| json parse | arm64 | 287,994,161 | 285,378,444 | 1.000 | 287,970,086 | 285,317,882 | 1.000 |
| json enc_small | arm64 | 185,504,825 | 150,869,420 | 0.777 | 185,486,782 | 150,839,937 | 0.776 |
| json enc_strings | arm64 | 233,438,221 | 191,900,273 | 0.797 | 233,422,831 | 191,874,384 | 0.796 |
| json small | llvm | 304,369,613 | 304,950,681 | 1.010 | 304,343,547 | 304,900,524 | 1.009 |
| json ints | llvm | 210,318,030 | 210,833,470 | 1.015 | 210,279,812 | 210,780,979 | 1.014 |
| json strings | llvm | 309,683,036 | 304,413,117 | 0.988 | 309,632,460 | 304,311,090 | 0.988 |
| json maps | llvm | 298,831,821 | 307,171,262 | 1.040 | 298,838,538 | 307,109,657 | 1.039 |
| json parse | llvm | 253,198,581 | 251,097,107 | 1.000 | 253,135,474 | 251,047,850 | 0.999 |
| json enc_small | llvm | 155,019,754 | 133,792,078 | 0.839 | 154,942,121 | 133,842,520 | 0.839 |
| json enc_strings | llvm | 208,997,141 | 182,753,367 | 0.860 | 208,976,517 | 182,698,859 | 0.859 |
| alloc small | arm64 | 292,157,166 | 292,203,822 | 1.000 | 292,126,153 | 292,162,589 | 1.000 |
| alloc strings | arm64 | 328,771,856 | 301,408,937 | 0.912 | 328,704,087 | 301,364,750 | 0.913 |
| alloc maps | arm64 | 301,850,513 | 303,588,038 | 1.006 | 301,816,237 | 303,523,982 | 1.006 |
| alloc mixed | arm64 | 470,964,384 | 472,579,552 | 1.003 | 470,909,566 | 472,544,596 | 1.004 |
| alloc grow | arm64 | 67,107,108 | 67,144,108 | 0.998 | 67,074,492 | 67,101,115 | 1.000 |
| alloc tasks | arm64 | 877,785,814 | 879,088,073 | 1.001 | 879,981,768 | 878,382,701 | 0.998 |
| alloc small | llvm | 245,936,719 | 247,911,876 | 1.009 | 245,894,990 | 247,890,480 | 1.009 |
| alloc strings | llvm | 308,542,136 | 284,994,174 | 0.920 | 308,482,720 | 284,910,345 | 0.920 |
| alloc maps | llvm | 268,285,557 | 267,127,537 | 0.996 | 268,278,683 | 267,128,092 | 0.996 |
| alloc mixed | llvm | 463,703,041 | 462,349,765 | 0.997 | 463,703,565 | 462,306,775 | 0.997 |
| alloc grow | llvm | 64,529,087 | 64,486,724 | 1.000 | 64,503,872 | 64,457,764 | 0.999 |
| alloc tasks | llvm | 782,169,134 | 783,162,679 | 1.001 | 777,764,612 | 777,628,616 | 1.000 |
| nested push | arm64 | 18,052,697 | 18,031,476 | 0.999 | 18,003,257 | 17,975,119 | 0.998 |
| nested strappend | arm64 | 19,076,833 | 18,936,538 | 0.993 | 19,145,336 | 18,915,455 | 0.988 |
| nested walk | arm64 | 31,721,711 | 31,721,830 | 1.000 | 31,679,409 | 31,705,161 | 1.001 |
| nested push | llvm | 14,997,946 | 15,131,751 | 1.009 | 15,057,582 | 15,057,475 | 1.000 |
| nested strappend | llvm | 16,062,199 | 15,985,431 | 0.995 | 16,049,835 | 15,908,298 | 0.991 |
| nested walk | llvm | 27,609,584 | 27,708,899 | 1.004 | 27,566,686 | 27,658,986 | 1.003 |

Every row is within decision 0180's bar: 0.776 to 1.040 (json maps on
LLVM, 1.039 and 1.040, its map encoding's strings; arm64 1.018). The
builders' callers gain most: json enc_small 0.776 (arm64) and 0.839
(LLVM), text debug 0.801 and 0.843, alloc strings 0.912 and 0.920:
the C's push_word and push_words went through a temporary buffer and
rs_append's claim, the Xo stores whole words into the unique string.

The self build (as stage 2, but each xo first rebuilt by itself so its
own code runs its std's objects; then each builds main's selfhost/xo
with main's std, XO_EMIT_JOBS=8, fresh build dir, caches off): A
106,490,249,251 and 106,814,699,400, B 106,260,312,041 and
106,757,715,666 instructions (0.998 and 0.999); the two outputs byte
identical. No leak count: xo ends with os.proc.exit, which skips the
report (it runs only when main returns).

Behavior on the Mac: internal/arm64 TestGoldenRun and internal/llvm
TestGolden (every golden program, LLVM against the Go backend and
want.txt, with the leak check), TestRuntime*, TestCABIDiff,
TestLeakCheck, TestHTTP1*.

### Next

- The other C creators of strings (xo_str_slice, xo_str_of_byte,
  xo_str_repeat, xo_str_join, the number text, xo_rs_append_rune and
  xo_rs_push_rune) as obj.xo exports called by generated code and
  std/native directly, then the C rs_new and its alloc_usable go.
- Delete the C forwarders (xo_rs_concat, xo_rs_append, the builders)
  once a seed emits the export calls and LLVM generated code calls the
  exports.
