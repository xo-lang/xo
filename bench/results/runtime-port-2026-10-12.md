# Runtime port, steps 1 and 2: number and time text, Str policy (2026-10-12)

Decision 0193. Machine: the shared 12 core Apple M series Mac (darwin
arm64), about ten agents running; every run through the heavy lock, one
at a time. Figures are instructions retired (`/usr/bin/time -l`, the
minimum of 5 runs of each workload), which are stable under load; wall
times are in the raw logs only and are not used.

## What was measured

`bench/perf/text/measure.sh` (`bench/perf/text`): each workload once per
process after building the same inputs (1,000 Ints, Durations, Times,
strings, rows). `none` is the setup alone. Workloads:
`parse_int` (200,000 Int.parse), `dur_fmt` (200,000 `"${d}"` of
Durations from ns to days), `dur_parse` (200,000 Duration.parse),
`time_fmt` (200,000 `"${t}"`), `time_parse` (50 std/json decodes of a
List of 1,000 Times), `quote` (Debug of 50,000 Str and 50,000 Bytes),
`split`, `lines`, `replace`, `trim`, `rune_count`.

Each table compares the default build (Xo path) with
`XO_LLRT_C=text,strings` (the C path: the same program calling the C
functions through text_c.xo and strings_c.xo). "ratio" is Xo over C for
the whole process (decision 0180's bar: at most 1.05); "less setup"
subtracts each path's `none` first. The setup itself is not the same
under both paths (it formats Durations and Ints), so "less setup" is an
approximation; the bar is read on the whole process, as in the pilot.

Checksums are identical on both backends and both paths for every
workload, and equal the Go backend's (the column of raw numbers in the
logs).

## Final (the committed code)

The tree at the last commit; the C primitives `rs_push_word` and
`rs_push_words` store whole little endian words.

| Workload | arm64 Xo | arm64 C | ratio | ratio less setup | LLVM Xo | LLVM C | ratio | ratio less setup |
|---|---|---|---|---|---|---|---|---|
| `none` | 21,107,445 | 24,520,743 | 0.861 |  | 18,867,245 | 22,328,045 | 0.845 |  |
| `parse_int` | 43,590,048 | 62,546,132 | 0.697 | 0.591 | 42,875,205 | 60,755,313 | 0.706 | 0.625 |
| `dur_fmt` | 149,469,471 | 129,062,386 | 1.158 | 1.228 | 132,479,054 | 118,539,562 | 1.118 | 1.181 |
| `dur_parse` | 182,552,307 | 229,091,297 | 0.797 | 0.789 | 144,587,738 | 216,930,075 | 0.667 | 0.646 |
| `time_fmt` | 182,004,917 | 872,041,558 | 0.209 | 0.190 | 158,681,946 | 864,148,410 | 0.184 | 0.166 |
| `time_parse` | 167,353,233 | 163,037,458 | 1.026 | 1.056 | 120,893,907 | 159,602,440 | 0.757 | 0.743 |
| `quote` | 144,306,454 | 280,704,167 | 0.514 | 0.481 | 135,407,787 | 277,021,908 | 0.489 | 0.458 |
| `split` | 99,579,606 | 103,050,001 | 0.966 | 0.999 | 85,227,006 | 88,651,351 | 0.961 | 1.001 |
| `lines` | 148,159,216 | 151,700,084 | 0.977 | 0.999 | 139,546,951 | 143,087,592 | 0.975 | 0.999 |
| `replace` | 82,462,759 | 107,975,442 | 0.764 | 0.735 | 74,013,337 | 108,696,997 | 0.681 | 0.638 |
| `trim` | 159,069,833 | 157,803,835 | 1.008 | 1.035 | 153,180,039 | 151,375,850 | 1.012 | 1.041 |
| `rune_count` | 77,013,106 | 80,313,985 | 0.959 | 1.002 | 74,425,690 | 77,752,127 | 0.957 | 1.002 |

The bar holds on both backends for parse_int (0.70x, 0.71x), dur_parse
(0.80x, 0.67x), time_fmt (0.21x, 0.18x: the C formatted with snprintf),
time_parse (1.03x, 0.76x), quote (0.51x, 0.49x), replace (0.76x,
0.68x), and trim (1.01x, 1.01x). split, lines, and rune_count are C on
both paths in this tree (step 2 keeps them C), so their rows only show
the setup difference. **dur_fmt misses it: 1.16x on arm64, 1.12x on
LLVM** (1.23x and 1.18x less setup).

## How it got there

First version (one `rs_push_word` call per short piece, split, lines,
and rune_count in Xo with `str_find`):

| Workload | arm64 Xo | arm64 C | ratio | ratio less setup | LLVM Xo | LLVM C | ratio | ratio less setup |
|---|---|---|---|---|---|---|---|---|
| `none` | 21,202,754 | 24,558,751 | 0.863 |  | 19,128,478 | 22,320,265 | 0.857 |  |
| `parse_int` | 43,762,096 | 62,490,576 | 0.700 | 0.595 | 43,138,759 | 60,766,834 | 0.710 | 0.625 |
| `dur_fmt` | 167,139,979 | 129,041,250 | 1.295 | 1.397 | 169,024,219 | 118,506,071 | 1.426 | 1.558 |
| `dur_parse` | 182,612,802 | 228,985,348 | 0.797 | 0.790 | 144,850,159 | 216,948,408 | 0.668 | 0.646 |
| `time_fmt` | 243,886,753 | 871,753,655 | 0.280 | 0.263 | 224,479,432 | 863,720,803 | 0.260 | 0.244 |
| `time_parse` | 167,500,386 | 163,028,323 | 1.027 | 1.057 | 121,216,886 | 159,537,212 | 0.760 | 0.744 |
| `quote` | 144,309,407 | 280,617,679 | 0.514 | 0.481 | 136,152,725 | 283,604,294 | 0.480 | 0.448 |
| `split` | 118,416,658 | 102,967,921 | 1.150 | 1.240 | 108,423,258 | 88,697,610 | 1.222 | 1.345 |
| `lines` | 200,508,448 | 151,649,342 | 1.322 | 1.411 | 185,689,493 | 143,080,533 | 1.298 | 1.379 |
| `replace` | 82,609,338 | 107,938,135 | 0.765 | 0.736 | 74,231,859 | 102,064,668 | 0.727 | 0.691 |
| `trim` | 159,234,595 | 157,853,137 | 1.009 | 1.036 | 153,473,647 | 151,470,230 | 1.013 | 1.040 |
| `rune_count` | 88,661,987 | 80,474,879 | 1.102 | 1.206 | 90,637,461 | 77,893,384 | 1.164 | 1.287 |

Second version: one `rs_push_words` call per Duration and Time text,
`str_index_byte` (memchr) for lines and one byte separators, a
branchless rune_count (a Str is valid UTF-8, so its runes are its non
continuation bytes):

| Workload | arm64 Xo | arm64 C | ratio | ratio less setup | LLVM Xo | LLVM C | ratio | ratio less setup |
|---|---|---|---|---|---|---|---|---|
| `none` | 21,385,256 | 24,525,283 | 0.872 |  | 19,181,316 | 22,287,367 | 0.861 |  |
| `parse_int` | 43,906,102 | 62,515,602 | 0.702 | 0.593 | 43,215,983 | 60,767,701 | 0.711 | 0.625 |
| `dur_fmt` | 167,831,873 | 128,999,697 | 1.301 | 1.402 | 150,804,413 | 118,587,564 | 1.272 | 1.367 |
| `dur_parse` | 182,766,351 | 228,927,137 | 0.798 | 0.790 | 144,901,627 | 216,933,086 | 0.668 | 0.646 |
| `time_fmt` | 216,457,449 | 871,745,777 | 0.248 | 0.230 | 193,440,095 | 863,812,658 | 0.224 | 0.207 |
| `time_parse` | 167,630,717 | 162,974,663 | 1.029 | 1.056 | 121,242,012 | 159,504,010 | 0.760 | 0.744 |
| `quote` | 144,574,632 | 280,587,017 | 0.515 | 0.481 | 135,810,539 | 276,906,426 | 0.490 | 0.458 |
| `split` | 105,486,346 | 102,988,960 | 1.024 | 1.072 | 96,744,383 | 88,671,873 | 1.091 | 1.168 |
| `lines` | 172,983,046 | 151,622,166 | 1.141 | 1.193 | 161,812,244 | 143,055,935 | 1.131 | 1.181 |
| `replace` | 82,786,312 | 107,937,343 | 0.767 | 0.736 | 74,258,579 | 102,035,649 | 0.728 | 0.691 |
| `trim` | 159,412,546 | 157,824,621 | 1.010 | 1.035 | 153,538,551 | 151,443,247 | 1.014 | 1.040 |
| `rune_count` | 90,696,873 | 80,439,848 | 1.128 | 1.240 | 72,030,503 | 77,865,920 | 0.925 | 0.951 |

split stayed at 1.02x and 1.09x, lines at 1.14x and 1.13x, rune_count
at 1.13x on arm64 (0.93x on LLVM): a C call to search and a C call to
copy each piece cost more than the C loop's memchr and copy, and the
Xo byte loop costs more than C's on arm64. They went back to C (and
`str_index_byte` was removed); the wasm runtime keeps its WAT versions.

Third attempt at dur_fmt: digits two at a time from a 200 byte `Bytes`
table: dur_fmt 1.39x and 1.40x, time_fmt 0.28x (both worse than the
plain loop: the index arithmetic and the table loads cost more than the
divisions saved). Reverted.

Fourth: the C primitives store whole words with `memcpy` (and in place
when the string is unique with 32 bytes of room) instead of a byte
loop: dur_fmt 1.30x to 1.16x on arm64 and 1.27x to 1.12x on LLVM (the
final table). A last variant (two digit fields without the loop, one
division a digit) gave dur_fmt 1.21x on arm64 and 1.09x on LLVM: worse
on arm64, so it was not kept.

Where dur_fmt's remaining cost is (arm64 disassembly of the benchmark):
`text.append_duration` is about 990 instructions of code with the digit
loop inlined at each field; each digit costs an `sdiv` and `msub` (the
arm64 backend does not turn a division by a constant into a multiply),
a checked shift (`lsl`, `asr`, `cmp` and a branch to `xo_fault_shl`),
and the loop's compare; `frac_words` and `ndigits` stay calls. The C
writes each digit into a stack buffer with no checks.

## Correctness

- Goldens `text_table` (166 lines: Int.parse edge cases, Durations of
  every unit and both signs including -2^63, Duration.parse of 45
  texts including the overflow edges and Unicode white space, Times
  from 1677 to 2262, std/json Time and Duration decoding, Debug of Str
  and Bytes with every escape) and `str_ops_table` (split, lines,
  replace, trims, and rune_count of 22 texts with 8 separators):
  identical on the Go backend, arm64, and LLVM, with the Xo path and
  with `XO_LLRT_C=text,strings`.
- internal/llvm TestGolden (with the leak check) and internal/arm64
  TestGoldenRun over text_table, str_ops_table, native_json,
  native_display, unicode_table, std_error_display, cli_parse,
  toml_basic, csv_basic, misc, fault_audit: pass.
- TestSelfhostSSA (`XO_SELFHOST_DIRS=text_table,str_ops_table,native_json,native_display,unicode_table`):
  5 runs, 335 functions identical at every stage (the builder change is
  mirrored in selfhost/ssa).
- The wasm backend (selfhost/wasm built with the Go backend, run under
  Node): str_ops_table, native_display, unicode_table, regexp_table,
  std_error_display, and misc produce want.txt, now through text.xo and
  strings.xo for Duration Display, Str Debug, replace, and the trims.
  text_table and native_json are still unsupported there for std/json's
  C only primitives (`json_parse`, `rs_append_json_str`), as before.
- `go test -short` over the repository.

## Sizes

- Runtime pack (`xo rtpack`, darwin/arm64, every variant and the TLS
  library): before (main at 9935b0d) 17,712 KiB and 17,118,160 bytes of
  objects, key 58a50ca7...; after 17,392 KiB and 16,795,920 bytes
  (322,240 bytes, 1.9 percent, less), key 195e2e19.... Built twice
  after: byte identical (reproducible). The ported C (338 lines under
  `XO_TEXT_C`, 74 under `XO_STRINGS_C`) is in no variant.
- C runtime: xo_std.c 1,353 lines before, 1,395 after (the guards and
  their comments); xo_bytes.c 739 to 789 (the two primitives). Xo:
  text.xo 619 lines, strings.xo 132, the C paths text_c.xo 33 and
  strings_c.xo 71.
- bench/perf/text binaries: arm64 417,688 bytes with the Xo path,
  400,696 with the C path (a locally compiled runtime with the C
  functions); LLVM 158,248 and 157,480.
