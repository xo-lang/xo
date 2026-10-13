# 0198: Delete the C paths of ported runtime pieces

Status: Accepted (2026-10-13, by the lead under the owner's decision
0195 direction). Implemented for unicode, strings, num, and jsontext;
the second round (2026-10-13, below) deleted tz, report, show, and the
C paths added since; text, sched, and net keep theirs.

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

## Second round (2026-10-13, after batch 8)

### Context

The lead's batch 8 full tier passed clean on main 99c5443d
(bench/results/batches-after-b-2026-10-13.md, 39 packages ok). It
covers decision 0193 steps 3 to 6 (tz, report, show, http1, hpack,
fspath, procspec), the portable group of decision 0195 (ints, glob,
digest, gen), the scan intrinsics (decision 0201), Atomic[T] (0200),
the reference counts and the allocator (0207), and the Linux and
platform layers under sysio (0206, 0213). Each of those kept a C path
behind `XO_LLRT_C=<name>`.

### Decision

Delete, as the first round did, the C path of every piece batch 8
covered: tz, report, show, atomic, fspath, gen, glob, digest, ints, rc,
alloc, scan, sysio, http1, hpack, and procspec. For each:
`std/native/llrt/<name>_c.xo`; the `-DXO_<NAME>_C` sections of std/llrt
(`XO_TZ_C`, `XO_REPORT_C`, `XO_SHOW_C`, `XO_ATOMIC_C`, `XO_FSPATH_C`,
`XO_GEN_C`, `XO_GLOB_C`, `XO_DIGEST_C`, `XO_RC_C`, `XO_ALLOC_C`, and
`XO_INTS_C`'s integer operations, floor division, and Str key hash);
the C only the C path reached without a flag (http1: the C parse
branches of hc_read_one, `xo_hc_xo` and the connection's `xo` field,
the server's text helpers `xo_url_parts`, `xo_url_error`,
`xo_query_parts`, `xo_path_unescape`, `xo_query_escape`, `xo_go_quote`,
`xo_canonical_key`, `xo_equal_fold`; hpack: the server's C HPACK calls,
`xo_h2_xo`, and its `dec` and `enc`; procspec: pr_parse's merge,
pr_dedup, pr_abs, pr_key_len, pr_w_quote); and the declarations in
std/native/rt.xo, rttime.xo, rtcrypto.xo, rttest.xo, and rthttp.xo
that only the deleted modules called.

Kept:

- **text**: Duration text misses the bar (decision 0194).
- **sched** (decision 0212) and **net** (decision 0206 stage 3): they
  merged after 99c5443d and have not passed a full tier.
- The C versions of the intrinsics (`int_wrap`, `u64_mul_hi`,
  `float_bits`, `float_from_bits`, `bytes_word`, `int_fits`,
  `u64_to_float`, `f32_round`, `i64_to_f32`), still compiled only with
  `-DXO_INTS_C` when an emitter calls one; the flag keeps its name.
- The C functions other C still calls: scan's (the C runtime's own
  string code), sysio's (wasm, darwin/amd64, the test world, and
  **Clock.now on every OS**, sysio.xo `clock_now`), the HTTP/1.1
  request parser (hp_head, hb_chunked) and the C HPACK, which the
  client uses, and the bytes_hex of to_hex.
- Declarations the SSA builders' calls without the native prelude
  still name (rt.int_op and friends, rt.gen_*, rt.prop_report,
  rt.rs_append_u64, rt.atomic_add, rt.crypto_sha256, rt.crypto_b64):
  the wasm runtime implements several; no native runtime has them now.
- Go side, left alone because decision 0215 deletes it: internal/build
  `IOFlags` and its tests, internal/check `LLRTUsesC` (it already
  ignores a name without `<name>_c.xo`), the SSA builder's C branches,
  and the Go tests that set `XO_LLRT_C` (they now build the Xo path
  twice). xo1's `io_flags` (selfhost/xo/link.xo) keeps the deleted
  entries, which add no flag now, because TestXo1IOFlagsSymbols
  requires the same symbols as `IOFlags`; both go with internal/build.

### Results

Lines (git numstat of commit 2fdcb1f7):

| File | Deleted | Added |
|---|---|---|
| std/llrt/xo_time.c | 880 | 3 |
| std/llrt/xo_crypto.c | 799 | 4 |
| std/llrt/xo_testing.c | 365 | 2 |
| std/llrt/xo_rt.c | 354 | 8 |
| std/llrt/xo_bytes.c | 251 | 6 |
| std/llrt/xo_proc.c | 244 | 12 |
| std/llrt/xo_fs.c | 178 | 0 |
| std/llrt/xo_http.c | 149 | 15 |
| std/llrt/xo_h2.c | 70 | 26 |
| std/llrt/xo_std.c | 9 | 0 |
| **std/llrt, runtime** | **3,299** | **76** |
| std/native/llrt `*_c.xo` (16 files) | 373 | 0 |

std/llrt's 15 files went from 29,916 to 26,693 lines. By piece, the
guarded sections (each `-DXO_*_C` applied to the parent alone): tz
868, digest 804 (xo_crypto.c 790, xo_bytes.c 14), ints 210 of 294,
glob 200, alloc 197, fspath 176, gen 99, rc 78, report 54, atomic 38,
show 37; unguarded: procspec 232, http1 134, hpack 44 (net of what
replaced them). The parent's sched and text sections are 481 and 514
lines.

Pack: the darwin/arm64 runtime pack (`xo rtpack`, Go built `xo`) went
from 15,020,450 to 14,752,778 bytes; its 32 runtime objects from
14,319,520 to 14,051,848 bytes (rt.o 397,896 to 390,816;
rt-time-fs-stdin-proc-tls.o 497,016 to 487,360), libxotls.a byte
identical. Unlike the first round the objects shrink: the http1, hpack,
and procspec C paths had no flag, so every pack compiled them. Each
pack built twice from two work directories is byte identical. The Go
built `xo` binary went from 31,818,434 to 31,719,202 bytes.

Tests, on the Mac (Apple M4 Pro, darwin/arm64, through the heavy
lock, commit 2fdcb1f7 plus the C test drivers fixed after it):
`go test -short` over the repository; internal/llvm TestNumDiff,
TestJSONTextDiff, TestTZDiff, TestIntsDiff, TestScanDiff,
TestDigestDiff, TestDigestTables, TestHPACKTables, TestHTTP1ParseDiff,
TestHTTPParseDiff, TestHTTPFuzz, TestH2Diff, TestProcSpecDiff,
TestSysioParity, TestTimeDiff, TestCABIDiff, TestRuntimeTasks,
TestRuntimeAlloc, TestRuntimeThreads pass; cmd/xo TestGlobDiff and
TestGenDiff pass; internal/llvm TestGolden and internal/arm64
TestGoldenRun and TestRun pass; xo1 built from these sources (87,285,784
bytes) passes scripts/selfhost-goldens.py with `--backend=arm64` (179
of 179) and `--backend=llvm` (179 of 179); internal/selfhost
TestSelfhostWasm passes; TestCryptoDiff passes after the change below.

Two Go tests needed the deleted C, and decision 0215 deletes both
packages, so the changes are the smallest that keep `go test -short`
green: TestCryptoDiff (internal/llvm, run in -short) feeds
crypto_diff.c SHA-256, HMAC, PBKDF2, Argon2, base64, and
verify_password lines, so the deleted digest C moved to
std/llrt/testdata/crypto_ref.c (804 lines), which only crypto_diff.c
includes and no runtime build compiles; it goes with that test.
internal/check TestLLRTCPath expected `XO_LLRT_C=all` to load show_c.xo
and six more; it now expects the Xo modules.

Found on the way: TestHTTPParseDiff failed and TestHTTPFuzz passed
while testing almost nothing, because their C drivers
(std/llrt/testdata/http_diff.c, http_fuzz.c) called xo_hc_read, which
now stops at every head for Xo's parser; they now play http1.xo's part
with the C parser the client keeps. A comment that changed the line
count of show.xo or prelude.xo broke internal/ssa's goldens
(gotchas/ssa-goldens-hold-llrt-line-numbers.md).

### Consequences

- `XO_LLRT_C` selects only text, sched, and net; any other name selects
  nothing. bench/perf scripts that compared a deleted C path now build
  the Xo path twice (a note in each); the results files keep the
  comparisons.
- The http1 and hpack servers have no C fallback; the client keeps the
  C parser and HPACK until it moves.
- A TestXo1IOFlagsSymbols cleanup (the dead io_flags entries) is due
  when internal/build goes.

## Consequences (first round)

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
