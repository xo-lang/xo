# Runtime port, step 5: Debug leaf text, zone rules, test report (2026-10-13)

Decision 0193 step 5. Machine: the shared 12 core Apple M series Mac
(darwin arm64), several agents running; every run through the heavy
lock, one at a time. Figures are instructions retired (`/usr/bin/time
-l`, the minimum of 5 runs of each workload). Native builds used the Go
driver (`XO_SELF_DRIVER=0`); the self hosted SSA builder makes the same
calls (selfhost/ssa show.xo and testmode.xo mirror build_show.go and
build_testmode.go).

## What was ported

- **Debug text of collections.** The Debug text of List, Map, Set,
  tuples, structs, and enums was already generated code (internal/ssa
  build_show.go, selfhost/ssa show.xo). Its C leaves were a Rune's
  Display and Debug (`xo_rs_append_rune`, `xo_rs_append_rune_quote`) and
  a UInt64's text (`xo_rs_append_u64`, an snprintf); they are now
  std/native/llrt/show.xo (`XO_LLRT_C=show` for the C path). The C
  rune_quote and u64 text moved under `#ifdef XO_SHOW_C`.
- **xo_bytes.c's sized integer text.** Int8 to UInt32 print through
  num.xo already (the same word as an Int); UInt64 now through show.xo
  (num.xo's append_int, and for 2^63 and above the quotient by 10 and
  one digit).
- **xo_time.c's zone rules.** std/native/llrt/tz.xo reads TZif data as
  Go's LoadLocationFromTZData, parses the POSIX TZ footer once at load,
  and looks instants up as Location.lookup (Go's cache of the zone in
  effect at load time included). A parsed zone is one List[Int]; the C
  keeps the zone data (`xo_tz_data`: the program's TZif data per
  constant name, the embedded zoneinfo.zip, or the host's files and
  $ZONEINFO zip, in time.LoadLocation's order) and the table of loaded
  zones (`xo_tz_find`, `xo_tz_keep`: the list held shared, freed at exit
  for the leak check). The zone data format and the zoneinfo.zip lookup
  are unchanged. `XO_LLRT_C=tz` loads tz_c.xo over the old C, now under
  `#ifdef XO_TZ_C`.
- **xo_testing.c's report text.** A property test case's report to the
  harness (`XO-PROP-CHOICES`, `XO-PROP-INPUT` lines, and the shortlex
  check against `XO_PROP_BEST` with `XO-PROP-NOT-SMALLER` and exit 3) is
  std/native/llrt/report.xo over four primitives (`gen_choices`,
  `test_write`, `prop_best`, `test_quit`); `XO_LLRT_C=report` for the C
  (`xo_prop_report` under `#ifdef XO_REPORT_C`). The pass, fail, and
  summary lines are the harness's (cmd/xo nativetest.go, selfhost/xo),
  not the runtime's.

## Final: bench/perf/text (the committed code)

"C" is the same compiler with `XO_LLRT_C=show,tz`. Five workloads are
new: `debug` (50,000 Debug texts of a struct with a UInt64, a Rune, a
List[Rune], a Map[Str, UInt64], and a tuple), `zone_offset` (198,000
offset_at in six zones, instants from 1900 to 2100: transitions, the
footer, and the cache), `zone_fmt` (48,000 time.format with RFC1123),
`zone_civil` (48,000 time.civil and back with time.instant), `zone_load`
(7,000 time.zone of six names and an unknown one, a run time name: the
whole zoneinfo.zip embedded). Checksums are identical on both backends,
both paths, and equal the Go backend's (debug 898330193, zone_offset
798227171, zone_fmt 38788427, zone_civil 207478926, zone_load
703324447). Setup (`none`): arm64 24,632,003 Xo, 24,635,786 C; LLVM
21,491,075 and 21,517,938.

| Workload | arm64 Xo | arm64 C | less setup | LLVM Xo | LLVM C | less setup |
|---|---|---|---|---|---|---|
| `debug` | 262,902,168 | 384,228,805 | 0.663 | 241,130,553 | 374,770,180 | 0.622 |
| `zone_offset` | 101,812,450 | 130,543,979 | 0.729 | 87,630,787 | 123,048,873 | 0.651 |
| `zone_fmt` | 915,102,540 | 927,125,257 | 0.987 | 830,023,915 | 844,339,171 | 0.983 |
| `zone_civil` | 250,245,206 | 285,822,812 | 0.864 | 226,209,135 | 270,839,465 | 0.821 |
| `zone_load` | 41,858,033 | 41,746,432 | 1.007 | 38,199,526 | 38,815,959 | 0.966 |

The bar (1.05) holds on both backends for every workload. The seventeen
earlier workloads, which do not use show.xo or tz.xo, are 0.999 to
1.002 of the C path in the same run (the check of the subtraction). The
setup grew by the step 5 inputs (`none` on arm64 21,947,201 at step 3,
24,632,003 now), so absolute counts moved; less setup they match step
3 (int_fmt on arm64 50,267,722 then, 50,293,813 now).

## How it got there

- First tz.xo kept the parsed rules in the native time.Zone as a
  struct (`Rules{id, d: Option[Data]}`, Data seven lists, a nested
  struct of the footer, six scalars) held in the C table as an rt.Any.
  arm64, less setup: zone_offset 0.781, zone_fmt 1.031, zone_civil
  0.922, zone_load 1.744 (LLVM 0.695, 1.006, 0.867, 1.609). The Zone had
  become 28 words passed by value through every time.xo helper, and
  rt.any_get copies the Any's shared list on every load
  (gotchas/wide-struct-by-value-copies.md).
- A parsed zone as one List[Int] (header, transitions, zones,
  abbreviations packed eight bytes a word), the C table holding the list
  itself: zone_fmt 0.987, zone_load 1.067 on arm64 (1.046 LLVM).
- The table holds only names that passed the name check, so a hit skips
  it (only `Local` without host data needs refusing): zone_load 1.007
  and 0.966.
- debug: the C printed a UInt64 with snprintf and allocated nothing
  else; the Xo UInt64 text is num.xo's word digits.

## bench/perf/report

`bench/perf/report/measure.sh`: report_test.xo's property tests `wide`
(five collections), `huge` (sixteen List[List[UInt64]], about 155
choices and 1.3 KB of input text a case), and `narrow` (one Int), 16
cases each (one process per case), the sum of each case's minimum of 5
runs; `0b` and `1b` with XO_PROP_BEST=0 (not smaller: exit 3). Output
bytes are identical on both paths.

| Test | arm64 Xo | arm64 C | ratio | LLVM Xo | LLVM C | ratio |
|---|---|---|---|---|---|---|
| `wide` | 265,424,460 | 266,615,947 | 0.996 | 232,988,018 | 233,150,707 | 0.999 |
| `wide`, best | 264,651,761 | 265,173,126 | 0.998 | 232,354,220 | 234,835,679 | 0.989 |
| `huge` | 266,382,126 | 267,273,461 | 0.997 | 233,667,623 | 233,901,384 | 0.999 |
| `huge`, best | 265,772,564 | 266,808,090 | 0.996 | 233,106,546 | 233,418,651 | 0.999 |
| `narrow` | 265,311,869 | 264,144,943 | 1.004 | 233,065,048 | 231,581,816 | 1.006 |

The bar holds on whole processes, but the report is below the noise: a
case process retires about 16.5 million instructions (runtime start and
exit), the same binary's sum varies by about 1 million (0.4 percent)
between runs, and `narrow`, which reports one choice on both paths,
differs by 0.4 to 0.6 percent. The report's own cost (155 choices) is
tens of thousands of instructions either way.

## Correctness

- TestTZDiff (internal/llvm/tz_test.go): every zone of Go's
  zoneinfo.zip (and six names that are not zones), each with instants
  one second around up to 40 of its transitions from 1700 to 2250, past
  the last (the footer, every third year to 2250 for a tenth of the
  zones), before the first, near now (the cache), and random: offset,
  abbreviation, RFC1123 in the zone, the civil time and its instant back
  with each Ambiguity, the local zone from TZ; 50,785 input lines. Go
  backend against arm64 and LLVM, Xo and C paths, with the embedded
  zoneinfo.zip, with --tzdata=system, and with --tzdata=system and
  $ZONEINFO naming the zip: identical.
- Goldens show_table (Rune and UInt64 at their bounds, every sized
  integer at its bounds, Debug of nested List, Map, Set, tuples, structs,
  enums, Option, Result) and tz_table (fourteen zones with historical
  rules, 30 minute and negative DST, from 1880 to 2240, the gaps and
  overlaps of 2026 with each Ambiguity, fixed zones, an unknown name):
  TestGolden (LLVM, with the leak check), TestGoldenSplit,
  TestGoldenShared, arm64 TestGoldenRun and TestGoldenRunShared, with
  the Xo paths and with XO_LLRT_C=all; also native_sized_ints,
  native_display, text_table, native_prelude, misc, str_ops_table,
  unicode_table.
- cmd/xo TestNoGCTests prop, same, test_logs (property cases, shrinking
  with XO_PROP_BEST, failures and skips, native against the Go
  backend), both paths.
- TestIOFlagsStep5, TestLLRTCPath, TestXo1IOFlagsSymbols.
- The darwin/arm64 runtime pack built twice: byte identical; rt.o
  431,088 bytes, rt-time.o 437,760.
- Not run here (the lead's merge and full tiers): TestSelfhostSSA and
  the other selfhost tiers, the wasm goldens.

## What stays C, and why

- The zone data and the table (`xo_tz_data`, `xo_tz_find`,
  `xo_tz_keep`, `xo_tz_now`, `xo_tz_local_name`): files, the embedded
  data, a mutex, and readlink, below what Xo expresses.
- The fault texts of sized integer arithmetic (`n_int_overflow` and
  friends in xo_bytes.c): `xo_int_op`, `xo_int_conv`, and `xo_rune_of`
  are C and C never calls compiled Xo; they move when sized integer
  operations do (decision 0193's crypto note).
- `xo_rs_append_rune`: BytesBuilder.write_rune's `rs_push_rune` calls it
  (Display of a Rune is show.xo).
- The literal pieces of Debug text (`str.concat` of `", "`, `[`, ...):
  copies, which item 2 keeps C; the cold `str.concat_int` of fault
  messages (slice bounds, a missing Map key, std/json errors) and
  fmt_go_g for `xo_fault_float_int`.
- XO-TEST-FAIL and XO-TEST-LOG framing (`xo_test_fail`, `xo_test_log`):
  writes of text the generated code made, then `_exit` or the record
  table; nothing to decide.
- The property generator (PCG, choices, replay): the generator, not
  report text.
