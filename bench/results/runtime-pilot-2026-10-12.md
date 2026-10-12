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

Bench programs (bench/perf/llvm/xo, arm64; minimum of 5 runs, cycles
noisy): see the final section's table below.
