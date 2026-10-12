# Runtime pilot: the Unicode operations in Xo (2026-10-12)

Decision 0180. Before: std/llrt with `xo_unicode.c` (C tables and C
users). After: `std/native/llrt/unicode.xo` and its generated tables, in
three steps (each a commit of the branch):

- **pure** (1df3176): everything in Xo, results built with
  `BytesBuilder.write_byte` (a C call per byte).
- **runs** (e8e2ed8): results built in a Str through `rs_*` intrinsics;
  unchanged stretches appended in one call (`rt.rs_append_bytes`, which
  also maps ASCII letters), the only new C.
- **opt** (5ea6880): direct tables for U+0080 to U+07FF (case mappings,
  class bitmaps), ASCII runs found by `rt.bytes_span` (a C scan).

Workloads (`bench/perf/unicode`, `<workload>` argument): `lower` and
`upper` are `to_lower`/`to_upper` of 468 KB of mixed case ASCII, 200
times; `lower_u` and `upper_u` the same on 254 KB of mixed scripts
(Greek, Cyrillic, Latin-1, CJK, halfwidth, Deseret), 200 times;
`eqlower` is `a.to_lower() == b.to_lower()` of both texts against their
upper case, 200 times; `classify` is `is_letter`, `is_digit`,
`is_space`, `is_print` over 231,000 runes of both, 10 times;
`runecase` is `Rune.to_lower` and `to_upper` over the same runes;
`quote` is `Str.quote` of 98 KB with controls, 20 times. Checksums are
identical on Go, arm64, and LLVM for every variant.

Machine: darwin/arm64 (Apple M series, 12 cores), Apple clang 21, about
ten agents running at once: times are noisy, instructions retired
(`/usr/bin/time -l`, minimum of 5 runs) are the figure to compare.
Best of 5 runs for times. Scripts: the session's `measure.sh` (build
each backend, run each workload 5 times).

## Instructions retired (minimum of 5 runs)

| Workload | Backend | C (before) | pure | runs | opt | opt / C |
|---|---|---|---|---|---|---|
| lower | arm64 | 859,485,249 | 9,376,399,705 | 2,398,996,347 | 575,092,821 | 0.67x |
| upper | arm64 | 913,076,767 | 9,358,676,708 | 2,332,510,199 | 575,460,189 | 0.63x |
| lower_u | arm64 | 4,950,836,189 | 18,508,120,466 | 14,636,933,906 | 7,162,969,711 | 1.45x |
| upper_u | arm64 | 4,901,904,208 | 18,587,623,896 | 14,584,001,070 | 7,110,888,914 | 1.45x |
| eqlower | arm64 | 11,476,435,065 | 40,102,705,016 | 33,090,289,460 | 18,600,524,768 | 1.62x |
| classify | arm64 | 289,700,855 | 866,525,542 | 866,222,040 | 422,221,187 | 1.46x |
| runecase | arm64 | 237,254,317 | 807,064,831 | 807,207,744 | 427,241,162 | 1.80x |
| quote | arm64 | 228,320,257 | 570,351,928 | 318,159,207 | 176,554,466 | 0.77x |
| lower | llvm | 861,499,127 | 8,271,260,101 | 1,877,199,280 | 577,586,386 | 0.67x |
| upper | llvm | 914,967,800 | 8,482,398,893 | 1,877,051,728 | 577,905,155 | 0.63x |
| lower_u | llvm | 4,958,173,067 | 12,640,595,061 | 9,237,267,704 | 5,077,790,898 | 1.02x |
| upper_u | llvm | 4,903,108,933 | 12,733,815,045 | 9,162,495,834 | 5,008,371,122 | 1.02x |
| eqlower | llvm | 11,477,214,484 | 27,107,153,172 | 20,980,569,129 | 12,922,577,922 | 1.13x |
| classify | llvm | 285,291,474 | 584,811,842 | 584,785,890 | 249,117,387 | 0.87x |
| runecase | llvm | 219,978,798 | 459,695,529 | 459,633,873 | 257,374,780 | 1.17x |
| quote | llvm | 180,422,106 | 455,367,084 | 236,446,829 | 141,852,999 | 0.79x |

For reference, the Go backend (unchanged by this work): lower
2,739,337,933, upper 3,277,420,771, lower_u 7,559,317,345, eqlower
17,179,869,675, classify 481,528,335, runecase 472,026,024, quote
298,783,564.

## Best time of 5 runs (ms, noisy)

| Workload | Backend | C | pure | opt |
|---|---|---|---|---|
| lower | arm64 | 46.9 | 435.1 | 32.9 |
| upper | arm64 | 47.4 | 469.0 | 33.0 |
| lower_u | arm64 | 222.5 | 637.0 | 282.3 |
| upper_u | arm64 | 218.2 | 652.4 | 283.5 |
| eqlower | arm64 | 484.5 | 1442.8 | 759.7 |
| classify | arm64 | 14.0 | 36.4 | 21.1 |
| runecase | arm64 | 10.2 | 31.1 | 16.8 |
| quote | arm64 | 7.5 | 22.1 | 5.7 |
| lower | llvm | 45.3 | 387.0 | 31.5 |
| upper | llvm | 45.8 | 406.3 | 31.5 |
| lower_u | llvm | 221.1 | 497.7 | 209.8 |
| upper_u | llvm | 214.9 | 505.7 | 213.1 |
| eqlower | llvm | 478.4 | 1148.1 | 537.4 |
| classify | llvm | 14.1 | 23.8 | 9.9 |
| runecase | llvm | 9.5 | 18.7 | 9.5 |
| quote | llvm | 7.5 | 17.9 | 4.3 |

## Why

- **pure** was 4 to 11 times slower: every byte of a result was a C
  call (`rs_push_byte`), and the arm64 backend's byte loops cost about
  14 instructions a byte (a bounds check and the data address computed
  in the loop, Bools materialized with `cset` and tested again).
- **opt** is faster than C on ASCII text and quoting: an ASCII stretch
  is one `bytes_span` scan and one `rs_append_bytes` copy, and Go's
  "a string with nothing to map" rule is gone (the C copied too).
- **opt** is slower on arm64 where Xo code runs per rune: each
  `Rune.is_letter` is now the prelude wrapper calling
  `unicode.is_letter` (two calls; before, the arm64 inliner inlined the
  wrapper, whose only call was the C function), and decode, to_case,
  and the binary searches are separate calls (the inliner takes only
  leaf functions, in one round). LLVM inlines across all of them (LTO)
  and is within 2 percent on text, 13 to 17 percent on the lower
  compare and Rune case mapping, and faster on classes.

## Experiment: the arm64 inliner run to a fixed point

Not committed: `inline` in internal/arm64/inline.go repeated (up to 4
rounds) until nothing changes, so functions that become leaves after one
round are inlined in the next. arm64 instructions, opt then experiment:
lower_u 7,162,969,711 to 6,891,728,671, upper_u 7,110,888,914 to
6,841,872,036, eqlower 18,600,524,768 to 18,058,236,052, classify
422,221,187 to 421,959,935, runecase 427,241,162 to 382,025,891, quote
and the ASCII workloads unchanged. 0 to 11 percent: the remaining gap is
the size limit (40 instructions, `is_letter` with its search is larger)
and the loop code, not the round count alone.

## The C path kept selectable (final tree, 4d2af44)

`XO_UNICODE=c` builds (std/native/llrt/unicode_c.xo over the C under
`-DXO_UNICODE_C`), arm64 then LLVM instructions: lower 859,536,438 and
861,811,440, upper 913,070,581 and 915,398,737, lower_u 4,951,345,973
and 4,953,675,153, upper_u 4,901,231,190 and 4,909,046,330, eqlower
11,484,586,229 and 11,474,544,132, classify 322,310,675 and 285,812,914,
runecase 337,595,366 and 251,668,361, quote 228,334,854 and 181,061,951.
The text workloads match the C before this work; on arm64 classify and
runecase pay 11 and 42 percent for the extra call through the runtime
module's wrapper (the same inliner limit). Checksums identical; the 19
Str goldens pass on LLVM and arm64 with the default path.

## Sizes

- Runtime object (`rt.o`, pack flags, no defines): 450,112 bytes before,
  424,864 after the pure step (text 266,761 to 243,909 bytes).
- Runtime pack (`xo rtpack`, darwin/arm64, 32 variants and the TLS
  library): 17,776 KiB and 16,546,544 bytes of runtime objects before;
  17,136 KiB and 15,769,664 bytes after (opt, and the final tree, whose
  C path is compiled only with -DXO_UNICODE_C, which no pack variant
  has): 776,880 bytes (4.7 percent) less. The pack key changes
  (e464bae6... before, 2d68bff2... final), as any edit of std/llrt does;
  packs stay reproducible (nothing about how they are built changed).
- C runtime: 26,240 lines before, 24,240 after (opt; `xo_unicode.c`'s
  1,783 lines and 259 lines of users removed, 42 lines of bulk
  primitives added). Xo: `unicode.xo` 405 lines, `unicode_tables.xo` 53
  lines (92 KB, generated).
- Program binaries (`bench/perf/unicode`): arm64 382,440 to 399,288
  bytes (+16,848), LLVM 140,408 to 156,888 (+16,480): the Bytes tables
  (25.0 KB of data, of which 11.5 KB the direct case tables) and the Xo
  code replace 16.7 KB of C arrays.

## arm64 inlining and bounds checks (decision 0184, 2026-10-12)

Decision 0184 (arm64-opt agent): the arm64 backend inlines small
functions that call others and larger ones with at most two call
sites, and drops the bounds, overflow, and shift checks that bounds,
loop conditions, and binary search midpoints prove. Same program and
workloads as above; `bench/perf/unicode/measure.sh` (committed with
this work: minimum instructions retired of 5 runs, `/usr/bin/time -l`)
under the heavy lock with other agents running. "Before" is the
compiler of 13e232e (the start of this work); "after" is 8df0ca7 (the
Go emitter; 50e8cee changes only the Xo twin). Each column is one
measure.sh run; the C path (`XO_UNICODE=c`) is measured with the same
compiler in the same run, because its Xo wrappers (unicode_c.xo,
prelude) are compiled by the backend too.

| Workload | arm64 before | C path, before compiler | arm64 after | C path, after compiler | after / C (same compiler) | LLVM (after run) |
|---|---|---|---|---|---|---|
| lower | 575,200,417 | 859,673,796 | 564,932,462 | 848,520,045 | 0.67x | 578,884,900 |
| upper | 575,212,543 | 913,179,811 | 565,843,842 | 902,605,029 | 0.63x | 579,728,424 |
| lower_u | 7,162,605,087 | 4,955,326,252 | 4,367,085,616 | 4,943,813,086 | 0.88x | 5,079,889,426 |
| upper_u | 7,110,835,220 | 4,901,523,516 | 4,318,602,582 | 4,891,830,369 | 0.88x | 5,009,481,077 |
| eqlower | 18,600,469,104 | 11,480,387,619 | 11,660,716,984 | 11,478,594,583 | 1.016x | 12,925,879,652 |
| classify | 422,164,664 | 322,473,521 | 298,370,210 | 290,586,770 | 1.027x | 249,886,094 |
| runecase | 427,392,940 | 338,207,582 | 247,965,624 | 278,447,190 | 0.89x | 258,131,929 |
| quote | 176,512,719 | 228,743,951 | 146,742,353 | 227,820,181 | 0.64x | 142,731,443 |

**Every workload is within 5 percent of the C path on arm64** (the bar
of decision 0180 item 7(b) for arm64): eqlower 1.6 percent and classify
2.7 percent above, the rest below. Against the C path as the old
compiler built it, classify is 0.93x and runecase 0.73x. The Xo code
retires 39 percent fewer instructions on lower_u and upper_u, 37 on
eqlower, 29 on classify, 42 on runecase, 17 on quote; ASCII lower and
upper 2 percent. arm64 now retires fewer instructions than LLVM on
lower_u, upper_u, eqlower, and runecase (LLVM is unchanged by this
work; LLVM's classify, 0.86x of arm64, stays ahead). Checksums are
identical on arm64 and LLVM in every run. Binary size (arm64):
399,288 bytes before, 399,176 after.

Steps (arm64, instructions; one run each, same workloads):

| Workload | before | 7b0b311: non-leaf inlining, bounds through operations, less facts, shift checks | final |
|---|---|---|---|
| lower_u | 7,162,605,087 | 5,798,496,726 | 4,367,085,616 |
| upper_u | 7,110,835,220 | 5,749,794,960 | 4,318,602,582 |
| eqlower | 18,600,469,104 | 14,899,883,518 | 11,660,716,984 |
| classify | 422,164,664 | 341,945,678 | 298,370,210 |
| runecase | 427,392,940 | 279,317,463 | 247,965,624 |
| quote | 176,512,719 | 156,618,574 | 146,742,353 |

After 7b0b311: midpoints in the range analysis (the CASE and class
searches lose every check), byte loads at constant offsets, constant
Bools threaded through joins, facts through `&&` joins, the few call
sites inlining rule (decode and to_case inlined into case_append; an
experiment with only that rule added measured lower_u 4,491,794,218,
eqlower 11,908,724,042), logical immediates, `liveDCE`.

Bench programs (bench/perf/llvm/xo, arm64): each binary built by the
before and the after compiler, run 5 times for the minimum, and the
pair measured twice in one heavy run (before, after, before, after),
millions:

| Program | instructions before | after | cycles before (two runs) | after (two runs) | binary bytes before / after |
|---|---|---|---|---|---|
| fib | 4,490.9 / 4,489.8 | 4,489.0 / 4,491.1 | 901.2 / 880.3 | 888.6 / 900.1 | 364,296 / 364,296 |
| sieve | 1,888.4 / 1,891.7 | 1,892.2 / 1,888.8 | 432.9 / 429.2 | 437.5 / 426.8 | 364,360 / 364,360 |
| nbody | 3,618.8 / 3,621.1 | 3,615.5 / 3,618.4 | 695.7 / 700.7 | 700.2 / 718.0 | 364,568 / 364,584 |
| sort | 1,465.9 / 1,470.4 | 1,448.8 / 1,445.0 | 923.7 / 936.7 | 920.0 / 913.2 | 364,440 / 364,440 |
| interp | 1,409.3 / 1,410.9 | 1,406.1 / 1,406.5 | 177.6 / 187.8 | 174.3 / 187.0 | 364,408 / 364,408 |
| collatz | 3,628.3 / 3,634.2 | 3,625.7 / 3,629.5 | 1,917.7 / 1,928.0 | 1,919.4 / 1,925.9 | 364,312 / 364,312 |
| strings | 987.6 / 989.8 | 986.7 / 986.9 | 148.7 / 155.1 | 145.5 / 151.5 | 364,328 / 364,328 |

The same binary's instruction count moves by up to 0.3 percent between
runs (kernel work under load) and its cycles by up to 3 percent, so
only sort's 1.4 percent fewer instructions is a measured change. nbody,
the program whose loop bounds checks decisions 0054, 0060, and 0075
kept, now has 9 bounds checks instead of 11 (and `bodies` and `energy`,
with one and two call sites, are inlined into main; `advance`, the hot
loop, is not): its
cycles are within the noise of these runs
(best 695.7 million before, 700.2 after, 0.6 percent; decision 0075's
placement sweep would be needed to tell a 1 percent shape effect from
placement).

### xo1's self compile and the fixed point (decision 0184)

`xo1` (selfhost/xo built by the Go compiler with the arm64 backend)
compiling selfhost/xo with `build --phase-times` under `/usr/bin/time
-l`, one run each, under the heavy lock (sources: main at 2982065 for
B0, the 50e8cee tree for B; each xo1 compiles its own sources, so B
compiles 8.5 percent more assembly):

| | B0 (before), run 1 | B0 (before), run 2 | B (after) | C (B compiles it again) |
|---|---|---|---|---|
| load average during the run | 8 to 11 | 10 to 50 | 4 to 10 | 6 to 10 |
| emit | 532.4 s | 2,205.8 s | 357.7 s | |
| inline phase | 8.5 s | 74.9 s | 7.3 s | |
| wall clock | 608.1 s | 2,353.7 s | 428.4 s | 467.7 s |
| user + sys CPU | 399.6 + 148.9 s | 436.2 + 473.3 s | 403.9 + 23.1 s | 414.7 + 40.6 s |
| instructions retired | 3,998.7 billion | 5,024.5 billion | 3,857.4 billion | 3,931.2 billion |
| cycles | 2,015.5 billion | 3,316.0 billion | 1,575.5 billion | 1,660.7 billion |
| assembly lines written | 14,127,137 | 14,127,137 | 15,324,015 | 15,324,015 |
| xo1 binary | 55,643,840 bytes | | 60,335,728 bytes | |

**The fixed point holds**: the Go emitter's assembly of the new
selfhost/xo (G), xo1's (B), and the rebuilt xo1's (C) are byte
identical (sha256 0ace77c2...99b0, 15,324,015 lines).

xo1's self compile now takes 1,575.5 billion cycles and 3,857.4 billion
instructions (B), against 2,015.5 billion and 3,998.7 billion for the
old xo1 compiling the old sources in the quieter of the two before
runs: 22 percent fewer cycles and 4 percent fewer instructions while
writing 8.5 percent more assembly. The kernel share moves with memory
pressure from other agents (run 2 of B0 ran at load averages up to 50:
473 s of sys time), so user CPU (399.6 s before, 403.9 s after, for
more output) and instructions are the safer figures; cycles are one run
each. The code growth is the cost of the few call sites rule: xo1's
assembly is 8.5 percent and its binary 8.4 percent larger.

Tests of this work: `go test -short` over the repository; the arm64
run tests (TestGoldenRun, TestRun, TestBench, TestInlineVerifies) on
50e8cee; TestSelfhostArm64 (go backend) on the runs under
`testdata/golden/`, `arm64/testdata/run`, `bench/perf`, `examples/`
(XO_SELFHOST_DIRS): 303 runs compared, 303 byte identical, 8,425
functions identical, 0 skipped; its 2 reported differences are two
vendor directories of internal/check's golden effect_ceiling_vendor
that the Xo checker rejects and the Go checker does not (not emitter
output).

## LLVM value ranges and inlining (decision 0192, 2026-10-12)

Decision 0192 (llvm-unicode agent): the LLVM backend runs decision
0184's range analysis with facts from dominating branches (also through
`&&` joins) and passed bounds checks, lowers the checked arithmetic it
proves as `nsw`, tells LLVM the interval of each binary search midpoint
(`llvm.assume`), and marks the calls of functions arm64's cost model
inlines `alwaysinline`. Same program and workloads; `bench/perf/unicode/measure.sh`
(minimum instructions retired of 5 runs) under the heavy lock with
other agents running. "Before" is the compiler of eb44c9b (the start of
this work), "after" the final tree; each was measured in its own run
(11:11 and 11:28), with the C path (`XO_UNICODE=c`) built by the same
compiler in the same run.

| Workload | LLVM before | C path (before run) | before / C | LLVM after | C path (after run) | after / C | arm64 before | arm64 after |
|---|---|---|---|---|---|---|---|---|
| lower | 577,397,763 | 861,883,530 | 0.670x | 560,262,645 | 844,680,328 | 0.663x | 563,990,681 | 564,011,334 |
| upper | 577,347,993 | 915,496,042 | 0.631x | 560,272,101 | 898,288,918 | 0.624x | 563,965,050 | 563,975,755 |
| lower_u | 5,077,549,843 | 4,959,172,264 | 1.024x | 4,052,792,153 | 4,946,611,782 | 0.819x | 4,365,382,601 | 4,365,400,404 |
| upper_u | 5,008,111,657 | 4,908,550,104 | 1.020x | 3,985,292,696 | 4,892,592,371 | 0.815x | 4,316,287,036 | 4,316,420,118 |
| eqlower | 12,922,283,786 | 11,478,464,233 | 1.126x | 10,885,971,027 | 11,479,936,507 | 0.948x | 11,658,863,567 | 11,658,878,117 |
| classify | 248,961,316 | 285,713,651 | 0.871x | 244,928,720 | 288,023,244 | 0.850x | 295,488,031 | 295,655,884 |
| runecase | 257,372,685 | 251,606,263 | 1.023x | 247,534,943 | 255,921,288 | 0.967x | 246,954,865 | 247,026,370 |
| quote | 141,822,796 | 180,955,176 | 0.784x | 130,472,526 | 180,040,972 | 0.725x | 145,759,862 | 145,806,634 |

**Every workload is now within 5 percent of the C path on both native
backends**, the bar of decision 0180 item 7(b): on LLVM every workload
retires fewer instructions than its C path (the closest, Rune case
mapping, 0.967x); on arm64 (unchanged by this work: its columns differ
by at most 0.06 percent between the two runs) the lower compare is
1.015x and the classes 1.023x, as decision 0184 measured. The LLVM
lower compare went from 1.126x to 0.948x; mixed script case mapping from
1.02x to 0.82x (20 percent fewer instructions). LLVM now retires fewer
instructions than arm64 on every workload but Rune case mapping (0.2
percent more). Checksums are
identical on both backends and both paths in every run.

Why (the hot loops' assembly against the C path's): the C path and the
Xo path both pass through clang with full LTO, so inlining across the
program and runtime boundary was not missing. The Xo binary searches
kept `b.vs` after `hi - lo`, the midpoint, `m * 12`, and `m + 1`, and
`b.hs` before every table read; the decoder kept the checks of `i + k`,
`i + 1`, `j + 1`; and `decode` and `to_case` stayed calls in
`case_append` (LLVM's inliner, threshold about 225, does not count call
sites), while C's `n_case_into` has decoding, case mapping, and encoding
inlined. The ASCII compare loop of `eq_lower` was already free of checks
on LLVM (19 instructions a byte in C, 21 in Xo: a byte zero extended
twice).

Experiments on the IR of the before compiler (scratch runs: minimum of
3, outside the heavy lock; instructions; the rows from "ranges, facts"
on are an intermediate version that also told LLVM the intervals of
loop parameters and had no `&&` join facts):

| Variant | lower_u | eqlower | classify | runecase | quote |
|---|---|---|---|---|---|
| before | 5,077,870,981 | 12,922,188,103 | 248,965,415 | 257,481,055 | 141,878,645 |
| every check of the unicode module removed (an unsound bound) | 4,043,621,012 | 10,830,376,394 | 199,459,843 | 218,335,448 | 112,439,504 |
| only overflow checks removed | 4,822,555,506 | 12,422,736,004 | 227,580,710 | 244,824,738 | 135,563,181 |
| only bounds checks removed | 4,759,096,104 | 12,282,759,131 | 226,632,650 | 245,320,210 | 132,431,076 |
| only shift checks removed | 5,077,947,865 | 12,922,678,280 | 249,019,454 | 257,432,821 | 141,899,389 |
| `alwaysinline` on decode and to_case, no facts | 4,631,818,918 | 12,001,216,753 | 248,982,141 | 264,072,248 | 132,643,381 |
| assumes on to_case's search by hand | 4,597,998,600 | 11,961,846,578 | 248,914,705 | 224,538,759 | 141,906,879 |
| ranges, facts, `nsw`, assumes (no inlining) | 4,542,146,789 | 11,885,091,042 | 242,865,060 | 222,214,087 | 139,356,210 |
| the same with `inlinehint` on decode and to_case | 4,542,740,414 | 11,886,577,653 | 242,579,894 | 188,729,305 | 139,374,833 |
| the same with call site `alwaysinline` on decode, to_case, rune_case and its forwarders (by hand) | 4,052,772,895 | 10,885,987,299 | 242,434,366 | 188,371,408 | 130,366,826 |
| final model with a copy budget of 600 instead of 500 | 4,052,904,674 | 10,886,235,950 | 244,856,913 | 188,439,640 | 130,380,357 |

Shift checks cost nothing (LLVM proves them); overflow and bounds checks
each cost about half of the gap, and only together do they let LLVM
inline the searches. With the arm64 budget of 500,
`prelude.rune_case` (264 instructions with `to_case`, 2 call sites) is
not inlinable, so `to_case` is forced into it and the wrapper stays a
call in the Rune loop: runecase 247 million (0.967x of C) instead of
188 million with a budget of 600. The budget is decision 0184's
constant for both backends and arm64's runecase is the same 247
million, so changing it is left to a joint measurement (next step).

Loop parameter assumes (an earlier version told LLVM the interval of
every Int parameter of a loop header) gained nothing on the unicode
workloads and made `bench/perf/llvm/xo` sieve retire 3,368,086,961
instructions instead of 2,085,790,420 (cycles 718 million instead of
432 million): LLVM unrolled the inner loop with a 128 bit trip count
computed by `__udivti3` (gotchas/llvm-loop-phi-ranges.md). Only
midpoints are told.

Bench programs (`bench/perf/llvm/xo`, LLVM, the before and after
compiler, 5 runs each interleaved, minimum; one run of the pair):

| Program | instructions before | after | cycles before | after | binary bytes before / after |
|---|---|---|---|---|---|
| fib | 3,822,569,575 | 3,822,519,621 | 886,035,916 | 889,777,294 | 105,608 / 105,608 |
| sieve | 2,085,337,633 | 2,085,321,687 | 391,165,851 | 403,130,385 | 106,040 / 106,040 |
| nbody | 2,624,600,571 | 2,624,522,556 | 729,509,809 | 729,853,481 | 106,296 / 106,296 |
| sort | 1,388,367,834 | 1,324,087,526 | 854,550,570 | 859,419,074 | 106,168 / 106,168 |
| interp | 1,613,297,160 | 1,613,318,435 | 254,663,036 | 255,759,527 | 106,072 / 106,072 |
| collatz | 3,617,540,859 | 3,617,645,990 | 1,991,122,960 | 1,934,666,918 | 105,752 / 105,752 |
| strings | 949,368,382 | 949,370,907 | 134,692,914 | 131,218,753 | 106,168 / 106,168 |

No change beyond noise but sort, 4.6 percent fewer instructions (cycles
within the run to run spread of these loaded runs, which reach 3
percent); binaries are the same size.

Tests of this work: `go test -short` over the repository; TestRangeFacts;
the LLVM goldens range_facts (new), bce_inline, unicode_table,
native_eq_lower, and 24 others with overflow, index, Str, list, and
fault cases; arm64 TestGoldenRun range_facts and bce_inline;
TestSelfhostLLVM (go backend, `XO_SELFHOST_DIRS=bench/,examples/,testdata/golden/,selfhost/lexer,selfhost/parser`):
297 runs lowered by both, 297 modules byte identical, 10,498 functions
identical, 0 differing; test binaries 42 runs, 42 modules byte
identical, 4,232 functions identical.
