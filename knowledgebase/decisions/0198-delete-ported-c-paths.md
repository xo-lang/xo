# 0198: Delete the C paths of ported runtime pieces

Status: Accepted (2026-10-13, by the lead under the owner's decision
0195 direction). Implemented for unicode, strings, num, and jsontext;
tz, report, and show wait (see Decision).

## Context

Decisions 0180 and 0193 moved runtime pieces from std/llrt's C to Xo
modules in std/native/llrt and kept each C path selectable
(`XO_UNICODE=c`, `XO_LLRT_C=<name>`, a `<name>_c.xo` over `#ifdef`
guarded C, an IOFlags entry that compiles the runtime locally) until
the Xo version met decision 0180's bar (within 5 percent of the C in
instructions on every workload) on both native backends. Decision 0193
item 5 says a later change deletes each C path once it does.

Decision 0180 item 7 asks for a new ADR before its unicode C path
(unicode_c.xo, the `XO_UNICODE_C` C, xo_unicode.c) is deleted, once
7(b) holds: bench/perf/unicode within 5 percent of `XO_UNICODE=c` on
every workload on both backends. Decision 0184 met it on arm64 and
decision 0192 on LLVM (every workload below the C path; item 7's
status line, bench/results/runtime-pilot-2026-10-12.md). This is that
ADR.

The zero-C audit (bench/results/zero-c-audit-2026-10-13.md, "C paths
kept for pieces already ported") counted 4,654 lines behind these
switches, 4,313 of them past the bar, and decision 0195's plan puts
their deletion first. The lead's batch 3 full tier on d2c87ca6 passed
(bench/results/batches-after-b-2026-10-13.md), which covers decision
0193 steps 1 to 4 and the unicode pilot.

## Decision

Delete, with their switches, flags, IOFlags entries (internal/build
`IOFlags` and xo1's `io_flags` in selfhost/xo/link.xo), C only tests,
and docs:

- **unicode** (decision 0180): xo_unicode.c, the `XO_UNICODE_C`
  sections of xo_std.c and xo_bytes.c, unicode_c.xo, `XO_UNICODE`
  (check.LLRTUsesC), the C table writer of TestUnicodeTables, and
  TestUnicodeCPath.
- **strings** (0193 step 2): the `XO_STRINGS_C` sections of xo_std.c,
  strings_c.xo.
- **num** (step 3): the `XO_NUM_C` sections of xo_std.c and xo_bytes.c,
  num_c.xo, `numText`'s C branch (internal/ssa), TestNumDiff's C
  subtests.
- **jsontext** (step 4): xo_json.c, the guarded
  `xo_rs_append_b64_quoted`, jsontext_c.xo, `jsonRT`'s C branch
  (internal/ssa; selfhost/ssa `json_rt` now goes through `llrt`, so
  without the native prelude it is unsupported instead of calling C
  that no longer exists), TestJSONTextDiff's C subtests,
  TestJSONParseDiff and std/llrt/testdata/json_diff.c, and http_fuzz.c's
  JSON part.
- `n_lower_eq` and `n_parse_float` (xo_std.c, unguarded): only the num
  and jsontext C paths called them.
- The declarations in std/native/rt.xo and rtjson.xo that only the
  deleted `<name>_c.xo` modules called (`str_trim`, `str_replace`,
  `float_parse_ok`, `float_parse`, `rs_concat_int`, `rs_concat_float`,
  the Unicode C section, `b64_*`, `bytes_hex_ok`, `bytes_from_hex`,
  `json_parse` to `rs_append_b64_quoted`). The C functions the emitters
  call directly (`xo_rs_concat_int` for `str.concat_int`) stay.

Kept: the C paths of **tz, report, and show** (decision 0193 step 5
has not passed a full tier yet), of **text** (Duration text misses the
bar, decision 0194), and the C versions of the inline intrinsics in
xo_bytes.c (`int_wrap`, `u64_mul_hi`, `float_bits`, `bytes_word`,
`int_fits`), the fallback for a call no emitter lowers inline; the
wasm backend has its own runtime, and the C ones go only once a grep
of every emitter's output over the goldens shows no call. `XO_LLRT_C`
keeps working for the modules left; the deleted names (and
`XO_UNICODE`) select nothing.

## Results

Lines deleted (git numstat of commit ec3c3553):

| File | Lines |
|---|---|
| std/llrt/xo_unicode.c | 1,783 |
| std/llrt/xo_json.c | 983 |
| std/llrt/xo_std.c | 421 |
| std/llrt/xo_bytes.c | 186 (188 removed, 2 comment lines reworded) |
| **std/llrt, runtime** | **3,373** |
| std/llrt/testdata/json_diff.c | 115 |
| std/llrt/testdata/http_fuzz.c (JSON part) | 38 |
| std/native/llrt `*_c.xo` (num 47, strings 71, unicode 41, jsontext 39) | 198 |
| std/native/rt.xo, rtjson.xo declarations | 59 |

The audit's figure for the same pieces is 3,354 lines (unicode 2,047,
jsontext 1,016, num 135, the shared 82, strings 74); the 19 more are the
blank and comment lines around the guarded sections. The whole commit
removes 4,298 lines and adds 92 over 36 files.

Pack size: the darwin/arm64 runtime pack (`xo rtpack`) is 15,938,754
bytes before and after (32 runtime objects, 15,237,824 bytes; rt.o
410,824), and every object is byte identical to the one before: packs
never compiled these `-D*_C` sections, and `-O2` already dropped the
unused static `n_parse_float`. Only rtpack.json's key changes (the
embedded sources). The pack built twice from two work directories is
byte identical. The Go built `xo` binary, which embeds std, went from
30,819,282 to 30,736,610 bytes (82,672 smaller).

Tests: `go test -short` over the repository; on EC2 (r8g.8xlarge, on
demand, linux/arm64) at ec3c3553: internal/llvm TestGolden (every
golden, LLVM against the Go backend and want.txt, leak checked),
TestNumDiff, TestJSONTextDiff, TestUnicodeTables, TestHTTPFuzz,
TestHTTPParseDiff; internal/arm64 TestGoldenRun and TestRun; xo1 built
from these sources through scripts/selfhost-goldens.py with
`--backend=arm64` and `--backend=llvm`: 172 of 173 programs pass on
each, the same as xo1 built from the parent acfc691f on a second
instance (show_table differs on both commits and both backends, so
it predates this change); bench/perf/unicode, json, and
text built on both native backends with the Go driver and with xo1,
the unicode workloads' checksums equal to the Go backend's.

## Consequences

- One implementation of each ported piece; nothing selects the deleted
  C, so the Xo path is the only one tested and shipped.
- A regression in these modules can no longer be bisected against the
  C path by an environment variable; git history has it (this commit's
  parent, acfc691f).
- The tables that duplicated runtime symbol lists (IOFlags and xo1's
  io_flags) are shorter; TestIOFlagsDeletedCPaths checks the deleted
  symbols turn on no flag.
- bench/perf/unicode and bench/perf/json lose their C baseline; earlier
  results files keep the comparisons.

## Alternatives

- Keep the C paths for comparison: rejected; decision 0193 item 5
  planned their deletion once the bar holds, and decision 0195's plan
  counts them as the first lines out.
- Delete tz, report, and show too: rejected for now; step 5 has not
  passed a full tier.
- Delete text's C path: rejected; Duration text is 1.171x on arm64 and
  1.087x on LLVM (decision 0194).
