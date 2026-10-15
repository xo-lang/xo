# The C that Xo compiles, refreshed: a finite checklist (decisions 0195, 0205, 0202)

Date: 2026-10-13. Main at d79ad79b (after decision 0198's second round,
decision 0206 stages 2 and 3, decision 0212 stages 1 and 2, decision
0213 stage 1, decision 0215). A refresh of
`bench/results/zero-c-audit-2026-10-13.md` (main at 3a13792a): an
analysis, no runs, no timings. Line counts are `wc -l` (raw lines,
comments and blank lines included). Every line of std/llrt was read and
put in one function group (boundaries at function starts, a group
carries the comment above it; 341 groups, checked to cover each file
with no gap or overlap by a script kept out of the repo per the repro
script rule), and each group in one bin:

- **(a) FALLBACK**: an Xo port is the default and the C is compiled only
  behind a switch. Sub-bins: the `XO_LLRT_C` switch table; C compiled
  only on demand (no emitter calls it); dead C (no caller in native
  builds, deletable now).
- **(b) NEEDS AN XO PORT**: compiled into a default build of some program
  without `use c` on some target. Kinds: PORT (logic to write in Xo),
  GLUE (wrappers, forwards to `xo_x_` exports, layout asserts, C helpers
  of other C: no new Xo, it goes when its C callers go), SHIM (a C library
  or POSIX substitute: xo_nolibc.c, xo_win.c).
- **(c) STAYS BY DESIGN**: `use c` support (decision 0205).
- **(d) THIRD PARTY**: Mbed TLS and fiat-crypto, replaced last by TLS in
  Xo (decision 0202).

## Totals

| What | 2026-10-13 audit (3a13792a) | Now (d79ad79b) |
|---|---|---|
| std/llrt (.c, no headers) | 30,669 | **26,743** |
| third_party/mbedtls/src .c | 120,810 | 120,810 |
| third_party/mbedtls/src .h | 74,707 | 74,707 |
| third_party/mbedtls/xo (xo_p256.c 810, xo_tls_accel.c 153, configs 125, win/pthread.h 84) | 1,172 | 1,172 |
| third_party/fiat-crypto/src (p256_64.c 2,011, p256_scalar_64.c 2,215) | 4,226 | 4,226 |
| **All C native builds compile** | 231,584 | **227,658** |

### By bin (std/llrt, 26,743 lines)

| Bin | Lines | Of which |
|---|---|---|
| (a) FALLBACK, `XO_LLRT_C` switch | 1,818 | sched 1,285 (xo_rt.c), text 533 (xo_std.c) |
| (a) compiled only on demand | 98 | `-DXO_INTS_C` intrinsics (xo_bytes.c) |
| (a) dead, no caller | 144 | xo_rt.c 49, xo_std.c 62, xo_bytes.c 33 |
| **(a) total** | **2,060** | |
| (b) NEEDS AN XO PORT | **24,678** | PORT 17,311, GLUE 4,868, SHIM 2,499 |
| (c) STAYS BY DESIGN | **5** | xo_std.c 23-27 `xo_cffi_str` |
| (d) THIRD PARTY | **200,915** | counted apart; all of it waits for decision 0202 |

### Per file

| File | Lines | (a) switch | (a) on demand | (a) dead | (b) PORT | (b) GLUE | (b) SHIM | (c) |
|---|---|---|---|---|---|---|---|---|
| xo_rt.c | 7,834 | 1,285 | 0 | 49 | 5,272 | 1,228 | 0 | 0 |
| xo_http.c | 3,925 | 0 | 0 | 0 | 2,239 | 1,686 | 0 | 0 |
| xo_h2.c | 3,583 | 0 | 0 | 0 | 2,665 | 918 | 0 | 0 |
| xo_proc.c | 1,911 | 0 | 0 | 0 | 1,828 | 83 | 0 | 0 |
| xo_nolibc.c | 1,614 | 0 | 0 | 0 | 0 | 303 | 1,311 | 0 |
| xo_tls.c | 1,386 | 0 | 0 | 0 | 1,135 | 251 | 0 | 0 |
| xo_testing.c | 1,321 | 0 | 0 | 0 | 1,321 | 0 | 0 | 0 |
| xo_win.c | 1,188 | 0 | 0 | 0 | 0 | 0 | 1,188 | 0 |
| xo_std.c | 866 | 533 | 0 | 62 | 131 | 135 | 0 | 5 |
| xo_fs.c | 828 | 0 | 0 | 0 | 809 | 19 | 0 | 0 |
| xo_crypto.c | 763 | 0 | 0 | 0 | 719 | 44 | 0 | 0 |
| xo_time.c | 520 | 0 | 0 | 0 | 461 | 59 | 0 | 0 |
| xo_bytes.c | 463 | 0 | 98 | 33 | 296 | 36 | 0 | 0 |
| xo_stdin.c | 435 | 0 | 0 | 0 | 435 | 0 | 0 | 0 |
| xo_sys.c | 106 | 0 | 0 | 0 | 0 | 106 | 0 | 0 |
| **total** | **26,743** | **1,818** | **98** | **144** | **17,311** | **4,868** | **2,499** | **5** |

What compiles where: xo_rt.c is the one translation unit; it includes
xo_win.c on Windows (xo_rt.c:37-61), xo_std.c, and xo_nolibc.c under
`XO_NOLIBC` (xo_rt.c:7827-7834); xo_std.c includes xo_bytes.c, xo_sys.c,
xo_http.c, xo_crypto.c, xo_testing.c always and xo_fs.c, xo_stdin.c,
xo_proc.c, xo_time.c under `-DXO_FS`, `-DXO_STDIN`, `-DXO_PROC`,
`-DXO_TIME` (xo_std.c:830-866; flags from selfhost/xo/link.xo:275-297);
xo_http.c includes xo_h2.c and xo_tls.c (xo_http.c:3921-3925); xo_tls.c
is real only under `-DXO_TLS` (stubs at xo_tls.c:1329-1386). wasm builds
compile no C.

## The switch table (verified)

`XO_LLRT_C=<name>[,...]` (or `all`) makes the checker load
`std/native/llrt/<name>_c.xo` in place of `<name>.xo`
(selfhost/check/load.xo:380-386; the names are read at
selfhost/xo/main.xo:244). Three `_c.xo` modules exist:

| Name | Module | -D flag it turns on | C it selects |
|---|---|---|---|
| sched | std/native/llrt/sched_c.xo (44 lines) | `-DXO_SCHED_C` when code names `xo_sched_c` (selfhost/xo/link.xo:395-397; internal/xotool/cc.go:137-138) | 1,285 lines of xo_rt.c (below) |
| text | std/native/llrt/text_c.xo (33 lines) | `-DXO_TEXT_C` (selfhost/xo/link.xo:298-307; cc.go:53) | 533 lines of xo_std.c (below) |
| net | std/native/llrt/net_c.xo (7 lines) | none: `net.raw()` answers 0 | the `NX_LIBC` branch at the top of each Linux `nx_*` wrapper (xo_rt.c:5452-5641); refused with XO_NOLIBC (selfhost/xo/main.xo:728-735) |

Stale plumbing found (Xo and Go, not C; deletable now, see W0):
selfhost/xo/link.xo:308-369 and 380-393 still map the symbols of the
deleted show, report, tz, atomic, fspath, gen, glob, digest, rc, and
alloc paths to -D flags that guard no C; its comment (308-315) keeps them
for TestXo1IOFlagsSymbols, which went with decision 0215 (no match under
internal/ or cmd/). internal/xotool/cc.go:55-109 and 119-133 mirror them.
Tests still set deleted switches, so they now build the Xo path twice:
internal/llvm/threads_test.go:205 (`-DXO_RC_C`), 234 and 246
(`-DXO_ALLOC_C`), ints_test.go:237, tz_test.go:217, digest_test.go:454,
scan_test.go:181, sysio_test.go:33, http1_diff_test.go:279.
TestLLRTCPath, which the earlier audit and decision 0198 cite, no longer
exists.

## (a) FALLBACK: the C kept for pieces already ported

### `XO_LLRT_C=sched`: 1,285 lines of xo_rt.c, delete after the next milestone run

Group ranges (each between `#ifdef XO_SCHED_C` and its `#else`/`#endif`,
or the `#else` arm of `#ifndef XO_SCHED_C`; 1,253 lines between the
guards, 1,285 with the comments that head them):

| xo_rt.c | Lines | What | Xo default |
|---|---|---|---|
| 175-177 | 3 | `tls_heap` as a C `__thread` | thread local word 3 (0212) |
| 1232-1240 | 9 | spin_relax | `rt.spin_hint` in pool.xo |
| 1241-1279 | 39 | futex_wait, futex_wake (`__ulock`, WaitOnAddress, SYS_futex) | pool.xo futex_wait, futex_wake |
| 1299-1312 | 14 | lock_slow | pool.xo lock_slow |
| 1418-1446 | 29 | park_reset, park_signal | pool.xo |
| 1467-1533 | 67 | node_link, node_unlink, cancel_locked, node_cancel, node_set_waiter | pool.xo node_* |
| 1589-1596 | 8 | `cur`, `tls_fib` as C `__thread` | thread local words 0, 1 |
| 2356-2441 | 86 | run queue ring | sched.xo |
| 2502-2506 | 5 | `tls_w` | thread local word 2 |
| 2540-2809 | 270 | worker pool: register, start (pthread_create), wakep, inject queue, steal, find_work | pool.xo |
| 2869-2873 | 5 | fiber declarations | sched.xo |
| 2901-2992 | 92 | park_wait_n | pool.xo park_wait |
| 3249-3267 | 19 | scope_units_done, claim_task | sched.xo |
| 3350-3378 | 29 | scope_prune | pool.xo scope_add |
| 3421-3634 | 214 | xo_fiber_switch, xo_fiber_start assembly (windows/arm64, arm64, windows/amd64, amd64) | emitted `rt.stack_switch`, `rt.fiber_start_addr` |
| 3766-3803 | 38 | fiber_frame | sched.xo |
| 3830-3985 | 156 | fiber_new, fiber_free (mmap, VirtualAlloc), fiber_main, park, wake, run_fiber | pool.xo, sched.xo |
| 4038-4057 | 20 | run_item | pool.xo |
| 4178-4226 | 49 | xo_sched_c (the C path's operations) | sched.xo |
| 4227-4303 | 77 | go_idle, worker_main | pool.xo worker_main |
| 4333-4356 | 24 | xo_task_spawn's C link and queueing | pool.xo scope_add |
| 4506-4537 | 32 | xo_scope_close's C help and join | pool.xo scope_join |

What proves the Xo path: TestRuntimeTasks and TestRuntimeEventLoop run
std/llrt/testdata/tasks.c and evloop.c on both paths under
ThreadSanitizer (internal/llvm/threads_test.go:262-268, 295-299);
TestGoldenTasks (threads_test.go:365) and every golden (conc_*, task_*,
fault_task*, net_echo, http_*: internal/llvm/testdata/golden, 153
programs) run the default path; the bar is in
`bench/results/task-switch-2026-10-13.md`. **Waits on a milestone
run**: sched stage 2 merged after the last clean Linux full tier
(batch 8 at 99c5443d, `bench/results/batches-after-b-2026-10-13.md:20`;
143 commits since), and its Windows paths (CreateThread with
SetThreadStackGuarantee, VirtualAlloc stacks with a PAGE_GUARD page,
WaitOnAddress) have run only in agents' batches. After a clean Linux
full tier and Windows batch on a main that contains them: delete the 22
ranges, sched_c.xo, `rt.sched_c` and its rt.xo declaration,
selfhost/xo/link.xo:395-397, internal/xotool/cc.go:137-138, and the
second flag set at threads_test.go:268 and 299.

**W1 done (2026-10-14, after the linux/arm64 full tier at 2fc5af45;
owner: arm64 only).** xo_rt.c went from 7,802 to 6,518 lines (1,293
deleted, 9 added: 1,284 net): every `#ifdef XO_SCHED_C` arm and the
`#else` arms of `#ifndef XO_SCHED_C` (`unifdef -UXO_SCHED_C`, then the
comments that named the C path). Also deleted: std/native/llrt/sched_c.xo
(44 lines), `rt.sched_c` (std/native/rt.xo) and its entry in
selfhost/ssa contract.xo, the -DXO_SCHED_C lines of selfhost/xo/link.xo
and internal/xotool/cc.go, and the -DXO_SCHED_C variants of
TestRuntimeTasks and TestRuntimeEventLoop (the Xo runs stay). With no
sched_c.xo, `XO_LLRT_C=sched` now fails the build (decision 0198, W0).
The comment edits in pool.xo, sched.xo, and cabi.xo keep their line
counts, since the arm64 assembly goldens hold their positions
(gotchas/ssa-goldens-hold-llrt-line-numbers.md). Verification on the
Mac (arm64): `go test -short` over the repo passes; `xo dev goldens`
"180 programs: 180 pass, 0 fail" on arm64 and on LLVM;
TestRuntimeTasks, TestRuntimeEventLoop, and TestGoldenTasks (36
programs under ThreadSanitizer) pass (TestGoldenThreads went with
decision 0215); scripts/bootstrap-xo.sh "fixed point: s2 and s3
binaries identical (90792576 bytes)", assembly identical (475632418
bytes), s1, s2, s3 59, 58, 58 s.

### `XO_LLRT_C=net`: no standalone C, nothing to delete with the switch alone

The libc path is one `NX_LIBC(call)` line at the top of each Linux
wrapper (xo_rt.c:5463-5641). It is not only the switch's: in a libc
Linux build of a program without the network exports (`xo_x_net_raw` is
weak, xo_rt.c:5465-5476, 5498-5510) the poller's epoll and the
descriptor calls take it by default, so the wrappers are counted in (b)
(item B23). Proof of the Xo path: TestResolverDiff against glibc
(internal/llvm/resolver_linux_test.go), bench/perf/net,
`bench/results/linux-syscalls-2026-10-13.md` ("Stage 3"). Stage 3 also
merged after batch 8: the Linux full tier is needed before net_c.xo
goes. Then, to delete the branches: make nx_on's poll and descriptor
calls unconditional (cabi_poll.xo is in every native program), and keep
the socket branches only until B23.

### `XO_LLRT_C=text`: 533 lines of xo_std.c (**deleted 2026-10-14**, decision 0227: the owner accepted Duration text at arm64 1.112 with LLVM 0.925; 514 lines of xo_std.c and text_c.xo deleted)

| xo_std.c | Lines | What |
|---|---|---|
| 276-290 | 15 | Int.parse |
| 291-434 | 144 | Duration text (n_fmt_duration, xo_rs_append_duration) |
| 435-497 | 63 | Time text |
| 498-553 | 56 | Debug quoting |
| 575-728 | 154 | Duration.parse |
| 729-731 | 3 | n_days_from_civil forward (only the C time parse calls it) |
| 732-829 | 98 | Time parse |

text.xo has been the default since decision 0193 step 1 (2026-10-12)
and is covered by batch 8; the golden text_table and every golden that
prints a Duration or Time run it. Nothing runs the C path any more. The
one blocker is Duration text: 1.112 of the C's instructions on the arm64
backend, 0.925 on LLVM (`bench/results/arm64-codegen-2026-10-13.md`,
"Duration text"; the 1.171 of const-div-2026-10-12.md is older). The
module has one switch, so all 533 lines wait for it (work item 9).

### On demand only: 98 lines of xo_bytes.c (`-DXO_INTS_C`)

xo_bytes.c 8-23 (u64_to_float, f32_round, i64_to_f32) and 211-292
(int_wrap, u64_mul_hi, float_bits, bytes_word, float_from_bits,
int_fits): compiled only when generated code calls one
(selfhost/xo/link.xo:370-379). Both emitters lower them inline when the
operation is a constant (selfhost/arm64/calls.xo:793-850,
selfhost/llvm/call.xo:498-546); `rt.int_fits` has no inline lowering but
no caller. Deleting them needs the SSA builders to reject a non-constant
`int_wrap` operation, `rt.int_fits` removed from std/native/rt.xo, and
the link.xo entry removed. No run needed beyond the goldens.

### Dead: 144 lines with no caller in native builds, deletable now

**B08 done (2026-10-14, sha512-rand-2026-10-14.md).** SHA-512 is
digest.xo (`sha512_3`, generated `sha512_block`), exported to the C
Ed25519 by std/native/llrt/cabi_crypto.xo (a weak C definition that
faults covers programs without std/crypto); its C, 129 lines, is
deleted. Rand is std/native/llrt/rand.xo, called as the native
prelude's llrt_rand_* (wasm keeps rt.rand_*), over linux.xo, darwin.xo,
windows.xo `random` and the test world's `xo_tw_seeded` and
`xo_tw_rand_u64` exports; xo_fs_raw_rand is deleted. The C Rand (95
lines of xo_crypto.c, 7 of xo_win.c) stays as seed compatibility: the
seed 0.9.0-5e21dc31 lowers Rand to it when it builds the checkout's xo
(gotchas/runtime-c-the-seed-still-calls.md); delete it after the next
seed. SHA-512 0.887 of the C's instructions on arm64, 0.641 on LLVM.
TestCryptoDiff (4,086 operations), goldens 180 of 180 on both backends,
seeded Rand identical to the Go backend, TestSelfhostWasm 133 pass, 21
unsupported, 0 fail of 154, and the fixed point from the prebuilt seed
(s2 and s3 identical, 88,878,128 bytes).

**W0 done (2026-10-14, commit 83eef9ee on the W0 branch).** The seven
groups are deleted: 141 lines (xo_rt.c 49, xo_std.c 59, xo_bytes.c 33;
the audit's 144 counted the blank lines between groups), with their
nine std/native/rt.xo declarations. b64_alpha and b64_val moved to
std/llrt/testdata/crypto_ref.c, the one remaining user (TestCryptoDiff).
selfhost/xo/link.xo io_flags and internal/xotool/cc.go IOFlags dropped
the -D entries of the deleted C paths (XO_SHOW_C, XO_REPORT_C,
XO_TZ_C, XO_ATOMIC_C, XO_FSPATH_C, XO_GEN_C, XO_GLOB_C, XO_DIGEST_C,
XO_RC_C, XO_ALLOC_C) and the symbols no C defines; the tests that set
deleted switches lost those variants (internal/llvm threads, ints, tz,
digest, scan, sysio, http1_diff; cmd/xo glob and gen_diff). xo now
fails a build on an XO_LLRT_C name with no std/native/llrt/<name>_c.xo
(cmd/xo TestLLRTCUnknownName). Verification on linux/arm64
(r8g.8xlarge, EC2 queue job w0-verify): selfhost-goldens.py with the
built xo, arm64 "179 programs: 179 pass, 0 fail", LLVM "179 programs:
179 pass, 0 fail"; nm finds 0 of the nine deleted symbols in the golden
binaries; TestSelfhostWasm ok; scripts/bootstrap-xo.sh "fixed point: s2
and s3 binaries identical (87478368 bytes)", s1 equals s2.

| Where | Lines | What | Why dead |
|---|---|---|---|
| xo_rt.c 7349-7397 | 49 | xo_utf8_valid | native builds call scan.xo utf8_valid (std/native/prelude.xo:130); the C is the fallback without a native prelude (selfhost/ssa/build.xo:2003-2012) |
| xo_std.c 103-112 | 10 | xo_str_contains, starts_with, ends_with | prelude.xo:30-34 route to scan.xo |
| xo_std.c 182-226 | 45 | xo_str_split, xo_str_lines | prelude.xo:44-46 |
| xo_std.c 568-574 | 7 | b64_alpha, b64_val | base64 is num.xo and digest.xo |
| xo_bytes.c 131-148 | 18 | xo_bytes_span | scan.xo span (decision 0201) |
| xo_bytes.c 196-198 | 3 | xo_bytes_rune_count | prelude.xo:124 |
| xo_bytes.c 199-210 | 12 | xo_bytes_hex | scan.xo to_hex (decision 0201) |

The wasm emitter names `rt.str_contains` and friends
(selfhost/wasm/call.xo:241) for its own WAT runtime, not this C. With
them go their declarations in std/native/rt.xo (14-16, 24-25, 43, 112,
114, 116) and the scan_rt fallback. Check before deleting: build the
goldens on both backends and grep the objects for the seven symbols.

## (b) NEEDS AN XO PORT: 24,678 lines as 31 work items

Every (b) group is in exactly one item below (the script checks it); the
ranges are the groups'. "Retires" is C lines; "Xo" a rough size of the
Xo to write (GLUE retires with no new Xo). Missing primitives are marked
MISSING.

| Item | Retires | Kinds | Ranges |
|---|---|---|---|
| B01 clocks off Linux (done 2026-10-14: the clocks in Xo, 27 lines net retired, the rest GLUE of later items; notes) | 132 | PORT 75, SHIM 57 | xo_rt.c 2022-2033, 6512-6566; xo_win.c 216-272; xo_time.c 500-507 |
| B02 runtime globals and thread locals (part 1 done 2026-10-14: rt.static_word, std/log state, 66 lines; part 2 done 2026-10-14: the Bytes primitives and the scratch in Xo, xo_sys.c 54 lines deleted, the `__thread` sys_scratch gone; notes) | 188 | PORT 72, GLUE 116 | xo_sys.c 1-106; xo_bytes.c 293-298, 366-375; xo_http.c 3072-3137 |
| B03 stdio, exit, env, args, stats (done 2026-10-14 in Xo: stdio-2026-10-14.md; the seed forwards and the env C go with the next seed; notes) | 343 | PORT 281, SHIM 62 | xo_rt.c 548-668, 1097-1145, 7398-7454; xo_std.c 554-567; xo_bytes.c 376-415; xo_win.c 1119-1180 |
| B04 xo_nolibc.c shrink (2026-10-14, decision 0230: stage (a) malloc in libcmem.xo; stages (b) to (d) mem*, str*, strerror, strto*, qsort in libcstr.xo, the system calls, errno, exit, signals, locks, threads in libcsys.xo and cabi_libc.xo; stage (e) the start, getenv, getauxval, vsnprintf, vfprintf, FILE, directories in libcio.xo, the threads and locks in libcthr.xo; UNTESTED on linux; C keeps _start's asm, environ and the std FILE pointers, the variadic printf shells: 128 lines) | 1,463 | SHIM 1,160, GLUE 303 | xo_nolibc.c 1-119, 223-1566 |
| B05 unwinder without libgcc (stage 1 done 2026-10-14, decision 0221: XO_NOLIBC linux/arm64; stage 3 done 2026-10-14 for glibc linux/arm64 and macOS, the libgcc path compiled only for glibc linux/amd64 until stage 2; notes) | 185 | PORT 137, SHIM 48 | xo_rt.c 1737-1777, 1880-1927, 1974-2021; xo_nolibc.c 1567-1614 |
| B06 fault record, fault exit, Windows unwind (done 2026-10-14: part 1 the record and fault_exit in fault.xo; part 2 the entry points as Xo exports, decision 0228, C forwarders until a seed; part 3 the Windows walk in unwind.xo win_raise; fault-record-2026-10-14.md; 2026-10-15, xo-fa: the root context of a thread running Xo outside any task is ctx.xo root (xo_rt.c ctx forwards; rt_once, rt_init, cur_set, park_init deleted). GLUE for the end pass, by generated code or the seed: xo_fault_*, xo_fault_take, xo_fault_msg and xo_fault_loc (C strings to Str), xo_landing, xo_is_canceled, xo_personality's forwarder; by platform: fault_raise's Windows context capture and the libgcc forced unwind and C personality of x86_64 with a C library (until decision 0221's stage 2), and the environ shim (xo_w_environ on Windows) C main hands to process.xo) | 319 | PORT 241, GLUE 78 | xo_rt.c 708-778, 1644-1736, 1778-1879, 1928-1973; xo_bytes.c 416-422 |
| B08 SHA-512 and rand (done 2026-10-14) | 258 | PORT 258 | xo_crypto.c 19-128, 142-289 |
| B09 constant time, crypto.equal, Ed25519 (done 2026-10-14: decision 0222; notes) | 505 | PORT 461, GLUE 44 | xo_crypto.c 1-18, 129-141, 290-763 |
| B10 HTTP/1.1 client parser, URL and text helpers (**done 2026-10-14**, http-client-2026-10-14.md: 1,629 runtime lines deleted, 265 added) | 1,558 | GLUE 1,277, PORT 281 | xo_http.c 153-273, 342-1397, 2972-3071, 3279-3457; xo_h2.c 1183-1284 |
| B11 client HPACK (**done 2026-10-14**, http2-client-hpack-2026-10-14.md: 786 runtime lines deleted, 242 added) | 583 | GLUE 583 | xo_h2.c 87-669 |
| B12 HTTP response writer (done 2026-10-14: http1w.xo; 514 lines deleted, 112 of glue added; http-writer-2026-10-14.md) | 537 | PORT 537 | xo_http.c 274-341, 1398-1866 |
| B13 string and list objects (stage 1 done 2026-10-14: http1.xo through scan.xo, xo_str_find deleted, the trace padding through scan.rune_count; stage 2 done 2026-10-14 for the list objects and the drop worklist (decision 0226, obj.xo, called from arm64 generated code without a C level; the C entries remain as forwarders for LLVM and the seed); stage 3 2026-10-14: the string objects' appends and builders (rs_append, push_byte, push_word, push_words, append_bytes, with_cap) in obj.xo, called from arm64 generated code and std/native without a C level, the C entries forwarding; 0.776 to 1.040 of main, std/llrt net 91 lines fewer; the C rs_new and the other C string creators stay; string-list-objects-2026-10-14.md; 2026-10-14 small files batch: str_byte, str_slice, str_join, str_repeat, str_of_byte, bytes_decode in raw.xo for std, xo_str_byte, xo_str_join, xo_str_of_byte, xo_bytes_decode, xo_rs_append_rune deleted; the show text's str_slice and str_repeat go to the prelude's llrt_ (xo_str_slice and xo_str_repeat stay for the seed and str_slice_at's callers); the next batch: the collection constants are consts.xo's (const_once, consts_free; table at the services record's word 144, the C entries forward), and `s[lo..hi]` on Str and Bytes goes to the prelude's llrt_str_slice_at and llrt_bytes_slice_at (the C xo_str_slice_at and xo_bytes_slice_at stay for the seed until the next one)) | 816 | PORT 797, GLUE 19 | xo_rt.c 395-446, 779-1096, 1146-1231; xo_std.c 80-102, 113-164, 227-268; xo_bytes.c 1-7, 24-130, 149-195, 423-463; xo_http.c 2931-2971 |
| B14 test world, part 1 (done 2026-10-14: testworld.xo, 916 lines deleted, 87 added; test-world-2026-10-14.md) | 1,059 | PORT 1,059 | xo_testing.c 1-211, 332-747, 995-1194; xo_rt.c 511-547; xo_stdin.c 59-106; xo_proc.c 148-221, 1201-1273 |
| B15 test world, part 2 (part 1 2026-10-14, the zero-C push: the harness channel, testworld.xo write_real and quit. Part 2: std/native/llrt/twrun.xo (its state hung off testworld.xo's record: no static word is free): FakeClock's sleeps, advance, and set, the in memory listeners, testing.run_main, FakeProc.signal_after's queue, os.proc.exit's unwind; xo_testing.c 617 to 252 lines. Part 3, **done 2026-10-15**: the test world's start (the variables, the tw record, SIGPIPE in entry.xo), the virtual clock's move (twrun.xo kick, with the stale verdict check), FakeProc.signal_after's timer (a timer whose function word is poll.xo FN_SIGNAL: poll.xo fire runs stop.signal and frees it, on the poller or the clock's move) and the end's releases are twrun.xo; MemFs's now is rt.clock_now; xo_testing.c deleted (252 to 0 lines), the C keeps the tw record, tw_is_fake, and the weak definitions of cabi_tw.xo's exports in xo_rt.c; the in memory listeners' C wrappers go with the network API to netconn.xo (xo lang 3). UNTESTED on linux and windows) | 494 | PORT 494 | xo_testing.c 212-331, 748-994, 1195-1321 |
| B16 Fs operations (fsops.xo; part 1, POSIX, done 2026-10-14: no C deleted yet; part 2, Windows, 2026-10-14: kernel32 from windows.xo, xo_win.c's Fs shims deleted (260 lines), xo_fs.c's operations compiled off Windows behind faulting link stubs; option D, 2026-10-14: macOS x86_64 through stat64 and __getdirentries64 (sysio layer 2), xo_fs.c's operations and xo_bytes.c's xo_fs_read deleted (831 lines net); the handle records, the status thread locals, and faulting link stubs stay; fs-ops-2026-10-14.md; 2026-10-14 small files batch: the status thread locals are sysio.xo's (thread local words 11 and 12), the test world's switches twflags.xo's (static words 32 and 33, from testworld.xo), Clock.now's scratch and getauxval (linux.xo, /proc/self/auxv; libcmem.xo with XO_NOLIBC) Xo, xo_sys.c 64 to 24 lines of wasm arm stubs; os.proc.exit in a test binary calls twrun.xo exit from the prelude; the next batch: the file and lock handles' records are fsops.xo's (raw_file, raw_lock, file_free, lock_free through the services record's new words 128 and 136), xo_fs.c 133 to 62 lines: Time's conversions, fs_canceled, and the wasm arm stubs) | 1,383 | PORT 876, SHIM 488, GLUE 19 | xo_fs.c 1-828; xo_bytes.c 299-365; xo_win.c 542-1029 |
| B17 contexts, budgets, shields (0212 stage 3; done 2026-10-14, decision 0229: ctx.xo and budget.xo, 287 lines deleted and 87 added; held_check and the root context stay C for B18 to B20; contexts-2026-10-14.md) | 454 | PORT 439, GLUE 15 | xo_rt.c 1534-1588, 1597-1643, 2034-2319, 6618-6683 |
| B18 wait lists, await, task record and end, scopes (0212 stage 3; part 1 done 2026-10-14: tasks.xo has the task's end, await, the failed task, s.cancel(), os.tasks, the wait entry in the context; part 2 2026-10-14: the spawn, the scope's open and close, through C service ops and the services record; part 3 2026-10-14: the free functions of tasks, scopes, mutexes, channels, selects and Atomics, and run_claimed (a heap context per run; the entry call stays C, sched_rt op 29 run_entry, so the unwinding's boundary is a C frame); still C: root_worker and pool_init (B20); tasks-2026-10-14.md) | 588 | PORT 571, GLUE 17 | xo_rt.c 3018-3248, 3284-3349, 3379-3390, 3986-4037, 4304-4332, 4357-4505, 4538-4586 |
| B19 channels, select, Mutex[T], Atomic object (Mutex done 2026-10-14: mutex.xo with held_check and held_del, the C keeps mutex_free behind the handle; channels done 2026-10-14: chan.xo, a waiting context's queue entry in xo_ctx (cent, fired); select done 2026-10-14: selects.xo, the C keeps select_free and the after arm's timer; the Atomic object done 2026-10-14: tasks.xo atomic_new; B19 done; the handles' free functions done 2026-10-14 in tasks.xo, mutex.xo, chan.xo, selects.xo) | 775 | PORT 775 | xo_rt.c 4587-5310, 6567-6617 |
| B20 pool and fiber remnants (part 2 2026-10-14: fibers_setup in poolsetup.xo; part 3 2026-10-14: the deadlock check, pool.xo dl_check, and the fault as cabi.xo deadlock; the dlk counts' record and tw_kick stay C, the counts were already pool.xo's; still C: pool_init, root_worker; scheduler block S1 2026-10-15, zc-b19: park_wait_n in tasks.xo (flush, budget pause, the root helper's record, the test world's counted wait), a helper's dequeue signals the joiner in pool.xo, rt.sched_rt ops 10 and 17 deleted, the dead C wait lists, mu_init, list_release, flush_pending, fib_get and eight cabi.xo exports only the C called deleted; xo_rt.c 3,809 to 3,665 lines; S2 2026-10-15: the pool record and its deadlock counts from ctx.xo init (the C's statics `pool` and `dlk` and their layout asserts deleted; the C reaches the record through svc.pool), 3,665 to 3,638 lines; still C: the fiber cache's and the services records (C reads fibers.stack and writes svc's function words: GLUE for the end pass), run_entry (C by design: the unwinding's boundary frame, decision 0085), the forwarders older xo's code calls (until the end pass's seed), and (2026-10-15, xo-fa, decision 0232) the sanitizer fiber hooks, the TSan edges, pthread_create and the fiber stacks' libc mmap are std/native/llrt/sanhook.xo over sys_c.xo (xo_sched_hook's body, tsan_worker, the ASAN and TSAN fiber macros, rt.sched_rt 8, 11, 12, 22, 23 and the sanitizer parts of 4, 5, 7 deleted); left: xo_sched_hook as a forwarder and the xo_<name> seed shims of sys_c.xo's functions and rt.sym_present (GLUE until a seed has decision 0232)) | 707 | GLUE 532, PORT 175 | xo_rt.c 1280-1298, 1313-1417, 1447-1466, 2320-2355, 2442-2501, 2507-2539, 2810-2868, 2874-2900, 2993-3017, 3268-3283, 3391-3420, 3635-3765, 3804-3829, 4058-4177 |
| B21 heap platform calls, C entry points (part 1 2026-10-14: chunks and alloc.xo's big blocks mapped from heapos.xo with a cache, the central lists' lock word, the C entries forwarding; rt.heap_large stays malloc as the runtime's record allocator; part 2 2026-10-14: the heap records of threads and their list from heapos.xo, heaps_mu and the dead spare list deleted; the C entry points (xo_alloc, xo_free, xo_resize, the forwarders) remain until generated code calls alloc.xo directly; 2026-10-15, zc-b19, entries E1: generated code calls heapos.xo's platform calls (rt.heap_big, heap_grow, heap_lock and the rest) through their exports on both backends, and on LLVM obj.xo's list and string entries too (generated-entries-2026-10-15.md); entries E2: string, box, closure environment and handle allocations and frees through obj.xo's exports on both backends; heap-platform-2026-10-14.md; the memory block, 2026-10-15: real-time sections (core 6.5, decision 0133) are Xo, the deferred free queue in obj.xo (rt_defer, rt_drain, so obj.xo no longer calls back into C in a section) and enter, exit with XO-F007, and a fault's leave in rtsec.xo; xo_rt.c 144 lines deleted, 24 added (rl_free_c, rc_defer_c, rl_free_rt, rc_defer_rt, rt_defer, rt_drain, stats_init gone; xo_realtime_enter and exit forward); what stays in the block is GLUE: the heap record's layout and the single threaded switch the C hot paths read, malloc for rt.heap_large, the generated code entries (xo_alloc, xo_free, the rc, list, and string entries, the fault entries until a seed names cabi.xo's), xo_str_eq and xo_str_cmp, and the number to string entries (their C buffers keep the interpolation's fast path); UNTESTED on linux and windows) | 389 | GLUE 262, PORT 127 | xo_rt.c 67-174, 178-394, 447-510 |
| B22 poller (poll.xo; 2026-10-14: timer heap, poller loop, and on Linux and macOS a descriptor's begin, end, close, free, and waits with a deadline in poll.xo, a pub API for other Xo code; C keeps pd_new's registration and Windows; stage 4 2026-10-14: on Windows too the descriptor's operations and waits are poll.xo's, and the AFD poll's arm, completion, and the completion port loop (poll_main_win), UNTESTED on windows; C keeps ev_init, pd_new's registration, and the WSAPoll fallback loop; remnants batch 2 2026-10-15, zc-b19: on Linux and macOS the event loop's start (the self pipe, kqueue or epoll, the poller thread through pool.thread_start), a descriptor's registration (pd_open, records from rt.heap_large), its shutdown and close, the poke, timer_add and timer_del, clock.sleep's wait, select's `after` timer and task.with_timeout's deadline (tagged timers FN_SELECT, FN_SCOPE) are poll.xo's and tasks.xo's; rt.sched_rt ops 24, 27 and 28 deleted; xo_rt.c 4,030 to 3,836 lines; UNTESTED on linux; Windows' start, registration and WSAPoll loop stay C for the Windows pass) | 920 | PORT 831, SHIM 89 | xo_rt.c 5311-5451, 5822-6511; xo_win.c 1030-1118 |
| B23 sockets off Linux, net API (netio.xo; stage 1 2026-10-14: macOS's socket calls through libSystem in sock.xo and its getaddrinfo in cabi_net.xo, the C's nx_ path on darwin; stage 2a 2026-10-14: Windows' socket calls through ws2_32 in sock.xo and its getaddrinfo in cabi_net.xo, the nx_ path on Windows, UNTESTED on windows; the deletion of the C library path and the Winsock shim next, after a Windows run; remnants batch 1 2026-10-15, zc-b19: Net.listen and dial (TCP and unix:), Listener.accept, addr, close, Conn.read, write, close, and the NetErr outcomes in std/native/llrt/netconn.xo, the C's network section one line forwarders (xo_rt.c 4,959 to 4,108 lines), the C library path (net.xo, net_c.xo, XO_LLRT_C=net) and xo_testing.c's in memory listener wrappers deleted, UNTESTED on linux and windows; the Winsock shim stays C for the Windows pass) | 1,304 | PORT 817, SHIM 269, GLUE 218 | xo_rt.c 5452-5821, 6684-7348; xo_win.c 273-541 |
| B24 standard input (stdin.xo; done 2026-10-14: the buffer, turn, lines, read_bytes, and error text in std/native/llrt/stdin.xo; xo_stdin.c keeps the descriptor's poller registration, a raw read, the waits, and the terminal calls; 2026-10-14 small files batch: the waits, the registration and raw read off Windows, isatty, the terminal width, echo, and set_tty's width are stdin.xo's too (Linux and Windows UNTESTED), xo_stdin.c 226 to 66 lines, Windows' pump registration and read only) | 387 | PORT 387 | xo_stdin.c 1-58, 107-435 |
| B25 processes (proc.xo) (stages 1 to 3 2026-10-14, the zero-C push: Proc.kill, FakeProc.killed, environ and cwd; the command start (posix_spawn on macOS, fork and execve on Linux UNTESTED) in proc.xo; one xo_proc_run; xo_proc.c 1,911 to 1,483 lines. Stage 4a: Proc.run, Cmd.output and run, and pipelines in proc.xo run and exec over poll.xo (pd_wait, the new sleep_until and canceled), Linux UNTESTED; the C's run, exec, collect, and drain compiled on Windows only, with faulting stubs elsewhere. Stage 4b: Cmd.spawn's children in proc.xo (spawn, write, close_stdin, read, read_line, wait, kill, fakes; the record allocated by a C shim whose free function calls proc.xo child_free); off Windows xo_proc.c is a 40 line shim, the rest compiled on Windows only (UNTESTED on Windows). Next: Windows CreateProcessW in Xo, then delete the C. Stage 5 2026-10-14: Windows in proc.xo too (launch_windows over windows.xo: exec.LookPath with PATHEXT, CreateProcessW with an explicit handle list, PIPE_NOWAIT pipes polled in steps instead of the C's socket pumps, process handles reaped with WaitForSingleObject), UNTESTED on windows; the prelude's rt.proc_* branch and xo_proc.c's Windows code deleted (xo_proc.c 1,431 to 24 lines: the child record's allocation)) | 1,764 | PORT 1,681, GLUE 83 | xo_proc.c 1-147, 222-1200, 1274-1911 |
| B26 signals, shutdown, entry (2026-10-14 part 1: the two phase shutdown's state machine, the stop request, the grace timer, task.stopping, proc.set_grace, main's root, and run_main's reset in stop.xo; C keeps the handlers, the entry, and a services record; part 2: the entry's start and end in entry.xo, the handlers' installation, win_init's arguments, the leak check, heap and runtime statistics' reports; C keeps the handlers, main's thread and fault boundary, _setmode; Linux and Windows UNTESTED) | 475 | PORT 372, SHIM 103 | xo_rt.c 7455-7826; xo_nolibc.c 120-222 |
| B27 zone data host side (tz.xo) (**done 2026-10-14**: std/native/llrt/tzhost.xo, xo_time.c deleted, 519 lines; notes) | 512 | PORT 453, GLUE 59 | xo_time.c 1-499, 508-520 |
| B28 HTTP/1.1 server loop, HTTP/2 server (**done 2026-10-14**, the zero-C push: part 1, the HTTP/1.1 connection (reading, the body, error answers, the response, a stream's chunks) in http1.xo, the C keeps socket and TLS I/O and the server's registration; TestHTTPParseDiff and TestHTTPFuzz skipped until Xo drivers; the frame writers and h2_head, frame checks, received field checks, request checks, SETTINGS checks are std/native/llrt/h2w.xo; part 2, the HTTP/2 server (the connection's fiber, frames, flow control, streams, responses, sending) is std/native/llrt/h2srv.xo, xo_h2.c deleted (2,068 lines), xo_http.c 1,284 to 1,079; the C keeps the TLS handshake and non-blocking reads, socket writes, the stream connections; zc-http, zc-h2; part 3, the server's registration, connections, sinks, streams table, request values and the client's dial, TLS start and close are std/native/llrt/hconn.xo, hserve.xo, hclient.xo, Any values anyval.xo, xo_http.c 1,079 to 115 (Mbed TLS wrappers, weak free functions, forwarders for generated code)) | 3,278 | PORT 2,705, GLUE 573 | xo_http.c 1867-2930; xo_h2.c 1-86, 670-1182, 1285-2899 |
| B29 HTTP client loops, HTTP/2 client (**in progress**: the HTTP/2 exchange is http1.xo h2_exchange, 2026-10-14, zc-http; the HTTP/1.1 client is hclient.xo, 2026-10-14: the connection's reads and writes, which the C's HTTP/2 client calls, the request, the reading side, the errors; the HTTP/2 connections, their pool (in the services record, ctx.xo SV_H2_POOL) and exchanges are hclient.xo too, 2026-10-14; still C: cl_start's dial and TLS handshake, and TLS itself) | 1,288 | PORT 1,274, GLUE 14 | xo_http.c 3138-3278, 3458-3920; xo_h2.c 2900-3583 |
| B30 TLS boundary pre-work (2026-10-14: done in tlscert.xo, 608 lines of Xo; xo_tls.c 1,386 to 1,073; C keeps the Mbed TLS calls behind tls_svc, the framework functions as its addresses, and the local zone offset; Linux and Windows UNTESTED) | 573 | PORT 522, GLUE 51 | xo_tls.c 122-559, 1036-1170 |
| B31 TLS in Xo (decision 0202; 2026-10-14 primitives 1 done: sha256.xo SHA-256, SHA-384, and SHA-512, hmac.xo, hkdf.xo with TLS 1.3's Expand-Label, 292 lines of Xo, golden crypto_prims; primitives 2: aesgcm.xo, constant time AES and AES-GCM (BearSSL's aes_ct64 and ghash_ctmul64), 570 lines, in crypto_prims; primitives 3: chacha.xo, constant time ChaCha20-Poly1305 (RFC 8439, poly1305-donna limbs), 299 lines, in crypto_prims; primitives 4: x25519.xo, constant time X25519 (RFC 7748 ladder on ed25519.xo's field), 141 lines, in crypto_prims; part 2, xo-4b: nistec.xo P-256 and P-384 ECDH and ECDSA, asn1.xo DER, x509.xo certificate parsing and signature checks, bigmod.xo and rsa.xo PKCS #1 v1.5 and PSS, 2,793 lines of Xo, goldens tls_nistec, tls_x509, tls_rsa against Go's crypto; part 3, xo-4b: x509verify.xo chain verification as Go 1.26's Verify, golden tls_verify; part 4, xo-4b: tls13.xo the TLS 1.3 client handshake and tlsrec.xo its record layer, 1,146 lines, golden tls13_rfc8448 (RFC 8448's trace); part 5, xo-4b: X25519 as the client's default share, the shared helpers for the server; part 6, xo-4b: golden tls13_go, traces of Go's TLS server (HRR, CertificateRequest, KeyUpdate, ECDSA P-384, Ed25519, ChaCha20, AES-256); part 7, xo-4b: the HTTP client's TLS is Xo's (tlsio.xo over tls13.xo; XO_LLRT_C=tlsio keeps Mbed through tlsio_c.xo); known gaps before merge: TLS 1.2-only servers refused until xo-fa's tls12 client (closed by part 8), macOS system roots through x509verify.xo not the Security framework; the server stays on Mbed until tls13srv.xo; size: the http_tls arm64 binary is 2,269,872 bytes on the Xo path, 1,753,344 with XO_LLRT_C=tlsio (+516,528, both TLS stacks linked), for the end pass's size and perf check; known issue, Mbed path only and on main too, not to chase: https://proxy.golang.org/ fails with a host name error ("certificate is valid for misc-sni.google.com, ..."), Mbed's SNI not reaching Google's front end; the Xo path gets 200; TLS 1.2 part 1, 2026-10-15, xo-fa: tls12.xo the TLS 1.2 PRF, key schedule with the extended master secret, AES-GCM and ChaCha20-Poly1305 records, and the client handshake (ECDHE over X25519, P-256 and P-384; ECDSA, RSA PKCS #1 v1.5, RSA-PSS and Ed25519 signatures; ALPN, SNI), 1,419 lines, golden tls12_client replaying crypto/tls's server against crypto/tls's own client as the reference (std/rt internal_tls12.go), 21 cases; version negotiation 2026-10-15: tls12.resume after tls13.xo's combined ClientHello, the downgrade sentinels, 24 cases; part 2, 2026-10-15: tls12srv.xo the TLS 1.2 server (resume after tls13srv.xo's HANDOFF), golden tls12_server against crypto/tls's own server, 21 cases; client certificates on the TLS 1.2 server are out of scope (lead, 2026-10-15); the TLS 1.2 server's ECDHE CBC-SHA suites (c009, c013, c00a, c014, as Go's defaults; sha1.xo, AES decryption in aesgcm.xo, Go's constant time padding and MAC check), server side first, then the client offering them as Go's (2026-10-15), goldens tls12_cbc, cbc_prims, tls12_client; performance against Mbed: bench/results/tls-perf-2026-10-15.md; the server next (tls12srv.xo); post-quantum 2026-10-15, xo-4: sha3.xo (Keccak, SHA3-256/512, SHAKE128/256) and mlkem.xo (ML-KEM-768 as Go's, constant time implicit rejection, X25519MLKEM768 helpers), tls13srv.xo takes X25519MLKEM768 first as Go's server, goldens crypto_mlkem (CCTV accumulated 100) and tls13_server (Go's default client), TestMLKEMAccumulated (CCTV 10,000); server checks 2026-10-15, xo-4: TestTLSServerReplayDiff (64K cases over 4 seeds, tls13srv and tls12srv against the Go references on LLVM and arm64) and TestTLSInteropMatrix (88 cells, the Xo and Go clients and servers crossed, Go's ML-KEM hybrid read as X25519); fixes: alert record version 0x0301 before the version, the ticket key slices (Go's ticketKeyFromBytes), TestTicketsAsGo against crypto/tls; session resumption 2026-10-15, xo-4: tlsticket.xo (Go's ticket format, sealing, 24 h/7 day keys) and TLS 1.3 PSK resumption in tls13srv.xo (tickets after Finished, psk_dhe_ke, constant time binder), golden tls13_server's Go client resumption traces, TestTLSDiff's resume tls 1.3 probe equal to Go; TLS 1.2 tickets are xo-fa's; client resumption out of scope (no ClientSessionCache in Xo); server wiring 2026-10-15, xo-4: tlssrv.xo runs the HTTP server's TLS on tls13srv.xo and, for TLS 1.2 clients, tls12srv.resume; http.tls_cert in Xo (PKCS #1/#8 RSA, SEC 1/PKCS #8 ECDSA, PKCS #8 Ed25519, Go's texts, golden http_tls_keys); XO_LLRT_C=tlssrv keeps Mbed (tlssrv_c.xo); out of scope and recorded: session tickets, client certificates, TLS 1.2 CBC; TLS 1.3 server 2026-10-15, xo-4: tls13srv.xo the TLS 1.3 server handshake as Go's handshake_server_tls13.go, 1,130 lines with its test op, golden tls13_server (RFC 8448 section 3 from the server side byte for byte, Go 1.26 client traces, Xo client and server against Go's crypto/tls); part 8, xo-4b: tls13.xo builds Go's combined ClientHello (TLS 1.2's suites, extensions, schemes when Conf.legacy_suites), the downgrade sentinel, HANDOFF to tls12.resume from tlsio.xo (no second try), ALPN h2 and http/1.1; TestTLS13ReplayDiff fuzzes the TLS 1.3 client against the Go port, 60,000 cases with no behavior difference, about 19 percent text only (x509.xo's parser laxer than Go's: next); part 9, xo-4b: x509.xo's parser a port of Go's parser.go over cbyte.xo (cryptobyte), x509verify.xo with Go's candidate order, alreadyInChain, unknown authority hint and check limit, macOS system roots through the Security framework (tlscert.xo apple_trust), TestX509ParseDiff 150,000 cases and 49 classes with no difference, the TLS fuzz's text bucket at 0 and failing above it; gaps: SAN URIs not parsed with net/url (closed by part 10), P-224 and P-521 signatures refused, IP/e-mail/URI constraints refuse rather than check; part 10, xo-4b: neturl.xo (http1.xo's net/url parser moved out, http1 unchanged), SAN URIs as Go's url.Parse with parseHost's texts, 61 classes, the TLS 1.2 hand-off on macOS through the Security framework; part 11, xo-4b: name constraints as Go's constraints.go (IP, DNS, URI, e-mail; Go's texts), TestX509VerifyDiff 20,000 random chains with no difference; gap next: certificate policy validation (Go's policiesValid); part 12, xo-4b: Go's policiesValid (RFC 9618 policy graph) and Verify's no valid chains text, TestX509VerifyDiff with policies, 20,000 chains with no difference; part 13, xo-4b: with_roots parsed once in Xo (tlsio.roots_pem), Mbed out of the Xo path's custom roots; Linux and Windows system roots still from Mbed's chain; part 14, xo-4b: expiry texts in the local zone (Go's time.Local) through xo_tls_c_local_off (C localtime; tz.xo's host lookup must be split from the embedded table before the end pass); part 15, xo-4b: tz.xo split (tzrules.xo, tztable.xo), the zone from Xo, xo_tls_c_local_off gone; part 16, xo-4b: Linux and Windows system roots in Xo (tlscert.system_ders, parsed once into static word 31), the Xo path free of Mbed's roots, UNTESTED on linux and windows; part 17, xo-4b: X25519MLKEM768 first in the Xo client as Go's (two key shares), tls13_go's hybrid trace, the TLS fuzz over it; removal plan for Mbed TLS and fiat-crypto: section (d) below; part 18, xo-4b: the default path at zero Mbed TLS and fiat-crypto (steps 1 to 3 of the plan: apple_trust on sys_darwin.xo's framework declarations, the system roots an Xo record, needs_tls only for XO_LLRT_C's C paths; nm of a default https binary: 0 mbedtls, fiat, psa symbols), XO_TLS_MLKEM=0 as Go's GODEBUG=tlsmlkem=0, golden tls_mlkem_off; part 19, xo-4b: Go 1.26's group list on the client (the three ML-KEM hybrids, X25519, P-256, P-384, P-521), ML-KEM-1024, P-521 ECDH and ECDSA, P-224 verification (the P-521/P-224 signature gap closed), traces of Go's server for each; P-521 fixed base table not taken: compiled in it costs every TLS build 27 s (arm64 29.6 s against 3.0 s) and 2.5 MB (5.0 MB against 2.5 MB), so pt_mul as P-384, a lazy table (Go's) as the follow-up once a static word slot registry exists; the Xo server's P-521 and hybrids next, xo-2b) | 813 | PORT 613, GLUE 200 | xo_tls.c 1-121, 560-1035, 1171-1386 |
| B32 xo_win.c, the Windows POSIX subset (2026-10-15, xo-fa, part 1: 914 to 484 lines; the shims nothing compiled for Windows still reached are deleted: files by path (stat, lstat, fstat, getcwd, the UTF-8 to UTF-16 path and Win32 errno helpers), open, fopen, pipe, fcntl, sysconf, nanosleep, sched_yield, gmtime_r, the socket, getpeername, shutdown, getsockopt, setsockopt wrappers, the string macros; the UTF-8 environment is process.xo win_env (GetEnvironmentStringsW) and the Windows poller's XO_WINPOLL lookup GetEnvironmentVariableA. Left, GLUE until their C users go: the threads (pthread_*, the mutex and condition types Mbed TLS's threading takes from here through XO_WIN_PTHREAD_H, pthread_once), Winsock's start and errno map with sock.xo's static asserts, the descriptor tags with close, read and write, the socket pair and the bind, listen, getsockname it uses (the C poller's WSAPoll fallback), localtime_r (xo_tls.c), and the stdin pump (part 2). Checked on the Mac only: -short, the http, net, fs, proc, stdin and env goldens on arm64 and llvm, and windows/amd64 and windows/arm64 cross links of the 15 http, net, fs, proc and stdin goldens (all link; toolchain.md "Windows cross builds on the Mac"). UNTESTED on windows: the end pass's windows CI run must cover those 15 goldens on both architectures, plus env_* and the XO_WINPOLL=wsapoll poller fallback, and a child process's environment (procspec.xo reads process.xo's environ)) | 430 | PORT 20, DEAD 410 | xo_win.c 1-914; xo_rt.c 2533 |
| BZ glue of the last caller (2026-10-15, xo-4 small files: Time conversions in std arithmetic, xo_time_of and xo_time_ns deleted; rt.fs_canceled is the prelude's llrt_fs_canceled; the wasm layer 0 calls (rt.fs_*) and rt.stdin_* off Windows lower to a fault on native targets (selfhost/ssa call.xo wasm_only_rt), so xo_fs.c, xo_sys.c and xo_stdin.c's stubs are seed compatibility only; BytesBuilder.write_rune is raw.xo push_rune; LLVM calls obj.xo's builder exports and both emitters cabi.xo's const_once as arm64 did; xo_std.c's dead helpers and cabi.xo's unused exports (utf8_decode, is_space, parse_int, fmt_duration, fmt_time, b64_val, days_from_civil) deleted; the end pass, labeled in the files: seed forwarders in xo_std.c (str_slice, str_repeat, rs_append), xo_bytes.c (builders, push_rune, slice_at, fault_bytes_index, const_once), xo_fs.c, xo_sys.c, the stubs of xo_stdin.c, and the B19 forwarders of xo_rt.c's channels, select, Mutex and Atomic (seed 0.11.0 predates B19); the Windows pump half of xo_stdin.c stays with xo_win.c) | 628 | GLUE 405, SHIM 223 | xo_rt.c 1-66, 669-707, 7827-7834; xo_std.c 1-79 less 23-27, 165-181, 269-275, 830-866; xo_http.c 1-152, 3921-3925; xo_win.c 1-215, 1181-1188 |
| **total** | **24,678** | | |

(B07 is not a (b) item: it is work item 9, the Duration text bar that
frees the 533 (a) lines.)

### Notes per item

- **B01 clocks.** Clock.now is C off Linux: sysio.clock_now gets 0 from
  `rt.sys_clock_buf` and calls `rt.clock_now` (std/native/llrt/sysio.xo:126-130);
  xo_clock_now is xo_rt.c 6512-6566, the Windows clocks are xo_win.c's
  clock_gettime (216-272). darwin.realtime_ns and windows.clock_ns exist
  (decision 0213 item 6). darwin/arm64 missed the bar only because the
  arm64 emitter does not fold `match target`
  (gotchas/arm64-match-target-not-folded.md); Windows is LLVM only, so
  it can be measured now. Also the monotonic clock (mono_ns, xo_rt.c
  2022-2033) that budgets and timers read. Xo about 100 lines, low.
  **Done 2026-10-14** (bench/results/clocks-2026-10-14.md): the clocks
  are Xo on every OS (sysio.xo clock_now, real_ns, mono_ns, the clock
  chosen once into word 0 of rt.sys_words); xo_win.c's clock_gettime
  shim (27 lines) and every C clock_gettime call are gone; std/llrt
  shrank by 27 lines net (46 deleted, 19 added). What stays is GLUE
  for later items: mono_ns and xo_tz_now as one line forwards to
  cabi.xo's exports (they go with their C callers, B17 to B22 and B27),
  xo_clock_now as the virtual clock of the test world (B14, B15) and
  the real time for xo_http.c's Date (B28), xo_sys_clock_buf (B02).
- **B02 globals and thread locals.** C `__thread` left: xo_bytes.c:296-297,
  xo_fs.c:15 and 224, xo_stdin.c:27-28, xo_http.c:1546-1547 and
  3101-3102, xo_sys.c:49, xo_rt.c:5626 (and xo_nolibc.c's errno). Thread
  local words 4 to 15 of decision 0212's table are free (word 4 is
  pool.xo's scratch). MISSING: `rt.static_word` (designed in decision
  0207, not in std/native/rt.xo) for process globals: the test world
  state, the zone table, std/log's state, the HTTP stream tables. Xo
  about 130 lines plus emitter work, low to medium.
  **Partly done 2026-10-14** (decision 0207, "Process globals as
  implemented"): `rt.static_word(k)` (64 words) is in std/native/rt.xo
  and the arm64, LLVM, and wasm emitters (wasm also lowers the thread
  local words now), with a 2 line C seed shim (`xo_static_word`,
  xo_sys.c) until the next seed. std/log's state is Xo
  (std/native/llrt/logstate.xo): static words 0 to 2, thread local words
  7 and 8 (words 5 and 6 went to B01's monotonic clock); the xo_http.c
  logging section (66 lines, with the `__thread` xlog_from) is deleted.
  **Part 2 done 2026-10-14** (decision 0207, "Moved in part 2"):
  bytes_addr, bytes_buf, rs_bytes_len are raw.xo over rt.cast (Bytes and
  Str are one SSA type) with no new intrinsic, and the per thread
  scratch is 256 bytes of the heap record (obj.xo scratch). Left: tw_fake, fs_set_err (the n_fs_*
  thread locals of xo_bytes.c and xo_fs.c), and auxval go with B14,
  B16, and B04/B26. B03, B13, B14, B16, B21, and B27 can use
  rt.static_word now.
- **B03 stdio, exit, env, args.** Stage 1 done 2026-10-14
  (bench/results/stdio-2026-10-14.md): the buffer, its flush, and
  standard error in std/native/llrt/stdio.xo, cabi.xo's `flush` (the
  `xo_x_flush` below) and `flush_try`; xo_rt.c's Output section 121
  lines to 39, of which the four xo_print forwards go with the first
  seed that has B03. Stages 2 and 3 done the same day
  (std/native/llrt/process.xo): exit, env.get, env.vars, env.args,
  env.program in Xo, argc, argv, and environ handed over by C main
  (cabi.xo `proc_start`); xo_exit is a one line seed forward, and
  xo_env_get, xo_env_vars, xo_env_args, xo_env_program stay as seed
  compatibility until the next seed (then about 70 lines go). Windows'
  UTF-8 environment copy (xo_win.c) stays for the C's own getenv. The
  original note follows. The 64 KiB stdout buffer (xo_rt.c
  548-668) needs an `xo_x_flush` export for the C fault and exit paths;
  env.get and env.vars (7398-7454) need the environment block (Linux:
  from the entry stack; darwin `_NSGetEnviron`; Windows
  GetEnvironmentStringsW, xo_win.c 1119-1180). Float text is already
  Xo for the C (num.xo through `xo_x_fmt_float`, xo_rt.c:676, 706); no
  C formats a float any more. Xo about 320 lines, medium.
- **B04 xo_nolibc.c** (XO_NOLIBC builds only). One line forwards to
  cabi_nolibc.xo already: signals 1457-1487, pthread 1488-1561, clock at
  1354. Next: malloc (223-389) to alloc.xo's exports, strerror (601-748)
  to linux.xo's table, file wrappers (1197-1335) to linux.xo; memcpy,
  memmove, memset, memcmp (in 390-600) must stay as exports under the
  plain C names, since LLVM emits calls of them
  (selfhost/llvm/call.xo:655-690). printf, FILE, strto*, qsort
  (749-1196) go as their C callers move (B03, B06, B13). Two items,
  medium.
- **B05 unwinder.** Stage 1 done 2026-10-14 (decision 0221, commit
  c38a15f8): std/native/llrt/unwind.xo has the .eh_frame_hdr lookup, the
  DWARF CFI interpreter, and the personality routine and LSDA reader as
  cabi_nolibc.xo exports (`xo_x_unwind_fault`, `xo_x_personality`, which
  the static link names xo_personality); rt.unwind_save and
  rt.unwind_resume are trampolines of the arm64 and LLVM emitters. With
  XO_NOLIBC=1 on linux/arm64 the 12 fault goldens pass on both backends
  (arm64 143 of 154 goldens, LLVM 144, the rest do not link). Retired:
  xo_nolibc.c 1567-1614 (its `_Unwind_*` stubs, 41 lines net). Left:
  x86_64 (decision 0221 stage 2: trampolines and register numbers), then
  the C library builds (stage 3: xo_rt.c 1737-1777 fault_stop and the
  forced unwind, 1880-1927 and 1974-2021 the C LSDA reader and
  personality, kept for glibc and macOS). Stage 3 done 2026-10-14
  (unwinder-glibc-2026-10-14.md): glibc linux/arm64 faults through
  unwind.xo (shared objects through ld.so's link_map; only frames whose
  personality is xo_personality are entered), and macOS through
  libunwind with the personality in Xo (unwind.xo darwin_personality;
  the stop function stays C, gotchas/libunwind-stop-must-not-return.md).
  The C fault_stop, forced unwind, personality, and LSDA reader are now
  compiled only for glibc linux/amd64 (XO_UNWIND_LIBGCC) and the LSDA
  reader for Windows, so no C lines are deleted yet (xo_rt.c +77 -16,
  most of it comments and the macOS and Linux fault_raise): they go with
  stage 2, which needs an amd64 machine to verify on. A 3 line C
  forwarder xo_personality stays until a seed's emitters name
  xo_x_personality (the emitters do since 2026-10-14, decision 0221
  point 7; the next seed after that lets it go). macOS arm64 then moved
  off libunwind too (decision 0221 point 10, the same day): unwind.xo
  reads __unwind_info, the C fault_stop and the macOS fault_raise are
  deleted, and faults cost 0.083 to 0.123 of libunwind's instructions.
  The original note follows.
  MISSING: a DWARF CFI unwinder over .eh_frame and
  .eh_frame_hdr in Xo, and a register restore and resume intrinsic;
  the personality routine (1974-2021) and LSDA reader (1880-1927) as a
  C ABI export named by `.cfi_personality`. Until then XO_NOLIBC faults
  end the program without deferred code (decision 0206 stage 2: 12
  goldens). macOS keeps libunwind (a platform library, decision 0195),
  so only the personality routine needs Xo there. Xo about 900 lines,
  high; three items (CFI interpreter; resume; personality and LSDA).
- **B06 faults.** Part 1 done 2026-10-14 (fault-record-2026-10-14.md):
  std/native/llrt/fault.xo keeps the record (set, detail, clear, take)
  and writes fault_exit's report, over xo_ctx offsets xo_rt.c asserts;
  the C keeps one line forwarders, fault_raise, xo_fault_msg and
  xo_fault_loc, and the entry points of generated code. Next: the entry
  points as Xo exports the emitters call (with C forwarders until a
  seed has them, as decision 0221 point 7 did for the personality), and
  the Windows walk. Part 2 done 2026-10-14 (decision 0228): generated
  code calls cabi.xo's xo_x_fault_raise_*, fault.xo writes the message
  and raises through fault_raise_msg; the C xo_fault_* are 3 line
  forwarders until the next seed. Part 3 done 2026-10-14: the Windows
  walk is unwind.xo win_raise (RtlVirtualUnwind through sys_windows.xo),
  the C walk and lsda_landing deleted; Windows wall clock 0.94 to 1.10
  of the C walk with a function entry cache. What B06 leaves in C:
  fault_raise (rt_fault_leave, the flags, the call of the unwinder),
  fault_raise_msg, xo_fault_msg and xo_fault_loc, the Windows
  personality stub, and the forwarders. The original note follows.
  The fault record (1778-1879), fault_exit (1644-1673),
  the Windows frame walk over RtlVirtualUnwind (1674-1736) through
  sys_windows.xo, its personality stub (1928-1973). Xo about 250 lines,
  medium; after B03 and B05.
- **B08 SHA-512 and rand.** SHA-512 (142-289) is data independent:
  portable now into digest.xo. rand (19-107) over linux.xo, darwin.xo,
  windows.xo random (exist) with a test world export of tw_rand_u64.
  Xo about 230 lines, low.
- **B09 constant time.** MISSING: decision 0195 prerequisite 5 (value
  barrier, constant time select, a check). Then crypto.equal (129-141;
  verify_password calls it, digest.xo:524, 541) and Ed25519 (290-737).
  Xo about 560 lines, high; two items. Prerequisite 5 and crypto.equal
  done 2026-10-14 (decision 0222: `rt.ct_barrier`, std/native/llrt/ct.xo,
  test/codegen/consttime, internal/llvm TestConstantTimeLLVM and
  TestCryptoXoDiff), Ed25519 too (std/native/llrt/ed25519.xo, 0.944 to
  0.971 of the C's instructions on arm64, 0.757 to 0.768 on LLVM,
  bench/results/ed25519-consttime-2026-10-14.md); xo_crypto.c is 16
  lines, `xo_ct_barrier` for the seed.
- **B10 HTTP client parser.** Since decision 0198 the C request head
  parser hp_head (xo_http.c 817-1278) is called only from hb_chunked's
  trailer parse (1368), and hb_chunked (1279-1397) only from the client's
  cl_read_response (3415). A `parse_response` in http1.xo beside
  parse_head (reusing line_at, canonical_key, has_token, trim_sp,
  chunked), driven by pause and resume as the server's parse is, retires
  cl_read_response (3279-3457) and with it 1,089 lines of C copies. The
  C URL parser (342-748) is still called by h2_request (xo_h2.c:1238,
  1242) and the client (xo_http.c:3721); http1.xo url_parts and
  query_parts replace it. Xo about 240 lines, low; two items. Proof:
  TestHTTP1ParseDiff (internal/llvm/http1_diff_test.go:242), http_fuzz.c,
  the goldens http_client, http_wire.
  **Done 2026-10-14** (`http-client-2026-10-14.md`): the client's loop
  is Xo (http1.xo client_do: URL, redirects, parse_response), the C
  keeps the connection, TLS, HTTP/2, and request writing (xo_cl_start,
  then xo_cl_input, cl_fill, cl_take, cl_end); the HTTP/2 server gets
  its :path parse from Xo (h2_target through h2_fields). Deleted from
  xo_http.c: the URL and query parsers, hp_head, hb_chunked,
  h_escape_path, h_split_hostport, cl_read_response, cl_once, xo_cl_do
  (3,925 to 2,543 lines); hp_head, hb_chunked, and the URL parser are
  test data in testdata/http_ref.c for http_diff.c and http_fuzz.c.
  Kept: h_tchar, h_lower, h_canon, h_server_token, h_has_token (B12 and
  xo_h2.c call them), h_valid_port, hurl, hp_cmp, h_go_quote (the
  HTTP/2 client's errors), h2_request's checks. Bar: client 0.953 to
  1.001, HTTP/2 server 1.006 to 1.008. Proof: TestHTTP1ClientDiff (457
  responses byte for byte as the C client).
- **B11 client HPACK.** xo_h2.c 87-669 is used only by the HTTP/2 client
  (its callers are in 2982-3444). hpack.xo has the decoder and encoder;
  it needs an encode one field entry (about 30 lines) and the client's
  hand off. Low.
  **Done 2026-10-14** (`http2-client-hpack-2026-10-14.md`): cl_start
  sets the HTTP/2 exchange up and Xo drives it (http1.xo h2_exchange:
  the field list, hpack.Enc.field, cl_h2_send, then cl_h2_next stops at
  each response block for hpack's decoder, cl_h2_end); the HPACK state
  is an rt.Any the C keeps with the pooled connection. xo_h2.c 3,606 to
  3,056 lines; h2len 0.982 to 1.006, h2big 0.996 to 1.009 of the C's
  instructions. The server's driver moved from hpack.xo to http1.xo
  (H2Conn) to keep the imports acyclic.
- **B12 response writer.** Status text, DetectContentType, Date, and the
  HTTP/1.1 writer rules (xo_http.c 274-341, 1398-1866): pure text, a new
  http1w.xo that HTTP/2's h2_head also uses. Xo about 500 lines, medium
  (the bar on the http workloads). **Done 2026-10-14**
  (http-writer-2026-10-14.md): std/native/llrt/http1w.xo (951 lines)
  through cabi_http.xo's five exports, weak C definitions that fault for
  programs without std/http; xo_http.c keeps wrappers that reserve the
  buffer room and pack the request's flags, and h_body_allowed (112
  lines added, 514 deleted); h2_head sniffs and dates through it; the
  C harnesses link the writer from a std/http program's IR (cOnlyHTTP).
  HTTP/1.1 workloads 0.976 to 1.001 of the C's instructions, HTTP/2
  within noise.
- **B13 string and list objects.** rs_new, append, concat, list new,
  copy, share, grow, free, handles, the drop worklist, real-time
  sections; xo_std.c and xo_bytes.c builders (push_byte, push_word,
  append_bytes, with_cap, slice checks, repeat, join), const_once.
  Two reroutes are trivial: http1.xo's `rt.str_find` to scan.find
  (retires xo_std.c 80-123), selfhost/ssa/show.xo:613's rune count to
  scan.rune_count (149-164). MISSING for the immortal objects:
  `rt.static_word`. Xo about 780 lines, medium; two items.
- **B14 and B15 test world.** Part 1 (records, generator draws, captured
  stdio and env, log records, FakeNet handlers, MemFs, cleanup, seeded
  rand, the stdin and process fakes) is portable once `rt.static_word`
  exists. Part 2 (virtual clock 212-331, in memory listeners 748-994,
  run_main 1195-1321) needs the timers (B22), wait lists (B18), and
  shutdown (B26). C reads `tw.on`, `tw.real`, `tw.virt` throughout
  (xo_rt.c 511-547). Xo about 890 plus 460 lines; three items.
  **Part 1 done 2026-10-14** (bench/results/test-world-2026-10-14.md):
  std/native/llrt/testworld.xo (static words 32 to 39 and a malloc'd
  record), the C's exports in cabi_tw.xo (test binaries only, weak C
  definitions elsewhere); xo_rt.c's `tw` struct stays, set by tw_init
  from Xo's answer. Left C for B15: the virtual clock, the in memory
  listeners, run_main, signal_after, tw_exit's unwinding.
- **B16 Fs operations.** Over the existing layers (linux.xo, darwin.xo,
  windows.xo; the FsErr tables are already Xo in sysio.xo). darwin/amd64
  still reaches xo_fs_stat, xo_fs_write, xo_fs_read (xo_fs.c 60-146,
  xo_bytes.c 299-365) because sysio.xo maps it to the C layer
  (sysio.xo:38-57). MISSING: a symbol name in a declaration for
  `stat$INODE64` (gotchas/darwin-amd64-stat-inode64.md); a timed wait
  from Xo for the polled flock (xo_fs.c 736-828). Retires xo_win.c's
  stat, dirent, and path shim (542-1029). Xo about 950 lines, medium;
  two items (POSIX; Windows).
  **Part 1 (POSIX) done 2026-10-14** (bench/results/fs-ops-2026-10-14.md):
  std/native/llrt/fsops.xo makes every Fs operation's calls from Xo on
  Linux and darwin/arm64 (the timed wait is `rt.clock_sleep_or_cancel`,
  the C's own wait declared in rt.xo); workloads 0.683 to 1.003 times
  the C path's instructions. No C deleted yet: Windows, darwin/amd64,
  and the unfolded `target` arms still reach xo_fs.c (it grew by 23
  lines: int64 record fields and the handle constructors
  xo_fs_raw_file and xo_fs_raw_lock, GLUE until Xo can make a record
  whose release runs Xo code). Left for part 2: the Windows calls
  (windows.xo) and xo_win.c 542-1029, then xo_fs.c 1-848 and xo_bytes.c
  299-365 (the status thread locals, which the C of the other targets
  writes) go; darwin/amd64 needs `stat$INODE64`, `opendir$INODE64`,
  `readdir$INODE64` names first (gotchas/darwin-amd64-stat-inode64.md).
- **B17 to B20 the scheduler's rest** (decision 0212 stage 3). MISSING:
  stack slots for wait records, or one wait entry per context (xo_ctx
  holds at most one wait). Contexts and budgets need the monotonic clock
  (B01) and Xo text (exists). B20 is mostly GLUE (records, layout
  asserts at xo_rt.c 4139-4177, `xo_sched_rt` services at 4058-4138)
  that goes as B17 to B19 land; Linux libc builds keep pthread_create
  until XO_NOLIBC is the default (decision 0206, "Default"). Xo about
  400, 580, 740, 200 lines; five items.
- **B21 heap platform calls.** heap_slow, heap_large, heap_chunk,
  heap_lock (xo_rt.c 190-316) over mmap and VirtualAlloc with Xo locks
  in place of pthread mutexes; the C entry points (317-394) go when
  generated code calls alloc.xo directly. Xo about 215 lines, medium.
- **B22 poller.** Timers, descriptors, kqueue (sys_darwin), AFD and IOCP
  (ntdll, kernel32), epoll (done through sock.xo), the poller thread,
  the Windows pumps (xo_win.c 1030-1118). MISSING: ntdll declarations
  and its import library on the link line (decision 0213, next steps).
  Xo about 790 lines, medium to high; two items.
- **B23 sockets and the net API.** darwin (libSystem socket,
  getaddrinfo: allowed by decision 0195) and Windows (ws2_32: MISSING
  declarations); listen, dial, accept, read, write, Unix sockets, the
  Winsock shim (xo_win.c 273-541). Xo about 875 lines; two items.
- **B24 stdin.xo.** Needs poller registration and waits from Xo (B22) and
  the Windows pump thread. Xo about 315 lines, medium.
- **B25 proc.xo.** darwin posix_spawn and its file actions through
  sys_darwin; Windows CreateProcessW with STARTUPINFOEX through
  sys_windows; Linux without libc MISSING a raw spawn (clone with
  CLONE_VFORK and an execve child path that makes only system calls, as
  Go's forkAndExecInChild). Needs B22 and B18. Xo about 1,275 lines,
  high; three items.
- **B26 signals, shutdown, entry.** MISSING: C ABI entries for platform
  callbacks (signal, console, vectored exception handlers) whose address
  Xo passes, a frame address intrinsic for the fault boundary, and an
  emitted `main`/`_start` (xo_nolibc.c 120-222). Xo about 440 lines,
  medium; two items.
- **B27 zone data host side.** MISSING: an intrinsic to address the
  xo_tzdata symbols (or xo1 emitting the table as an Xo constant),
  readlink in sys_darwin.xo, GetDynamicTimeZoneInformation in
  sys_windows.xo (the Windows table, xo_time.c 82-344, needs a Windows
  run). Xo about 380 lines, medium.
  **Done 2026-10-14**: no intrinsic was needed. The zone data the build
  writes beside the program (selfhost/xo tz.xo tz_source for C, ld.xo
  tz_object for Mach-O and ELF) now also defines `xo_tz_table`, a
  function returning the address of a four word table (mode, zip, its
  length, blobs), which rttime.xo declares (a bodiless rttime function
  is a call of `xo_<name>`). std/native/llrt/tzhost.xo has the rest in
  Xo: the zone data per mode, the host's files ($ZONEINFO as a directory
  or a zip, the system directories, /etc/localtime), the table of kept
  zones (static words 18 to 21, a lock, a vector of name blocks and
  shared List[Int] zones), the exit free for the leak check (cabi_time.xo
  `tz_free`, hooked in xo_std.c under XO_TIME), the /etc/localtime link
  name, and Windows' "Local" from GetDynamicTimeZoneInformation with Go's
  abbreviation table. std/llrt/xo_time.c (519 lines) is deleted. The
  compiler does not use std/time, so the 0.11.0 seed was unaffected.
  Verified: TestTZDiff (embedded, system, zoneinfo) on arm64 and LLVM,
  the goldens on both, a leak check of a zone loading program (0 live),
  CI auto and windows (runs 38638196744, 38638200760), and Windows'
  "Local" against the Go backend's time.Local under six system zones
  (scripts/ci/tzlocal-windows.sh, run 38640206795): every sample is the
  same except times before the first of Go's transitions in zones with
  daylight saving (Pacific, Morocco at 1906): Go's initLocalFromTZI
  writes transitions for the years now - 100 to now + 100 and uses the
  first standard zone before them, while the TZif made here (as
  xo_time.c's did) has none and extends the rule to every year. Fixed
  the same day (branch tz-win-local): tzhost.xo writes Go's two
  transitions a year for the 100 years each side of now with no footer,
  and takes the abbreviations in Go 1.26's order (the standard name,
  then the key name for Go's registry lookup, then the capitals without
  a default); scripts/ci/tzlocal-windows.sh run 38642276054: eight
  system zones (both hemispheres, half hour, no daylight saving) the
  same as the Go backend at nine times from 1843 to 2128.
- **B28 and B29 HTTP loops.** The HTTP/2 frame reader, writer, and
  sending (xo_h2.c 670-2335) are bytes only and can move before the
  loops; the connection loops need await (B18), the poller (B22),
  sockets off Linux (B23), and TLS (B31) for their `tls_*` calls; the
  stream and pool tables need `rt.static_word` (B02). Xo about 2,400
  and 1,100 lines; six items.
- **B30 TLS pre-work.** Portable before TLS in Xo: Linux root files
  (xo_tls.c 491-559), Windows roots through crypt32 (477-490), macOS
  roots and SecTrust verification through Security.framework and
  CoreFoundation declarations (413-476, 1101-1170), rfc3339 text
  (1036-1100), the X509KeyPair error texts (in 173-321). Xo about 535
  lines; two items.
- **B31 TLS** (decision 0202), with the third party below, last.
  Primitives first, one file each in std/native/llrt (owner, 2026-10-14):
  SHA-256/384/512, HMAC, HKDF (done, sha256.xo, hmac.xo, hkdf.xo); AES-GCM
  constant time (done, aesgcm.xo: BearSSL's aes_ct64 and ghash_ctmul64);
  ChaCha20-Poly1305 (done, chacha.xo); X25519 (done, x25519.xo); P-256 and P-384 (done,
  nistec.xo), DER and X.509 (done, asn1.xo, x509.xo), RSA (done,
  bigmod.xo, rsa.xo); then chain verification and the TLS 1.3 and 1.2
  protocol. Tested through
  internal.crypto_op against Go's crypto packages and the RFC vectors
  (internal/llvm/testdata/golden/crypto_prims).
- **BZ** file headers, includes, nstr, nbuf, n_str_list, the
  forwards to cabi.xo, xo_win.c's pthread and once shim over SRW locks
  (70-215; Mbed TLS's threading uses it too): no Xo; each goes with its
  last C caller.

## (c) STAYS BY DESIGN: 5 lines

xo_std.c 23-27, `xo_cffi_str`: copies the Str results of `use c`
adapters; only the generated adapters call it
(selfhost/cli/bind/gen.xo:397, 478). Decision 0205 keeps `use c` and its
C toolchain. It calls `nstr` (BZ), so it keeps its own copy when nstr
goes. Outside std/llrt, also by design: the generated `cffi/xo_<prefix>.c`
adapters (selfhost/xo/cffi.xo:41-46) and the C files `use c` programs
link (examples/realtime-control/rtclock.c, 16 lines).

## (d) THIRD PARTY: 200,915 lines, last

Mbed TLS 4.1.1 (third_party/mbedtls: src .c 120,810 in 118 files, .h
74,707, xo/ 1,172) and fiat-crypto (4,226), compiled into libxotls.a by
selfhost/xo/tls.xo for programs whose code calls the std/http server or
client (`-DXO_TLS`). Unchanged since the first audit. Replaced by TLS
and its crypto in Xo after Go's design (decision 0202), after the
constant time prerequisite (B09); the C path stays selectable until the
Xo TLS passes test vectors, interoperability, differential tests against
Mbed TLS, and fuzzing (decision 0202, Consequences). XO_NOLIBC refuses
TLS programs until then (selfhost/xo/main.xo:722-727).

### Removing Mbed TLS and fiat-crypto: the plan (B31, xo-4b, 2026-10-15)

Nothing is deleted yet: the owner decides at the end pass, after the
speed review (bench/results/tls-perf-2026-10-15.md: the Xo handshake's
instruction count against Mbed's).

**Status 2026-10-15 (part 18): steps 1 to 3 below are done; the default path links no Mbed TLS and no fiat-crypto (nm of http_tls on arm64 and LLVM). The rest of this section is the plan as written before them.**

**On the default (Xo) path.** Client and server run in Xo (tlsio.xo over
tls13.xo and tls12.xo; tlssrv.xo over tls13srv.xo and tls12srv.xo).
Parsing, verification, key exchange (X25519MLKEM768, X25519, P-256,
P-384), signatures and record protection are Xo. What still reaches
xo_tls.c (so the build still compiles it with `-DXO_TLS` and links
libxotls.a):

1. `rthttp.tls_c_apple_svc` (tlsio.xo, macOS with the system's roots):
   it runs `tls_init`, which starts Mbed TLS (the PSA crypto and
   threading set up), only to hand back `tls_svc`, the table of
   Security framework function addresses tlscert.xo's apple_trust
   calls through. Next step: build the table in a C file compiled
   without Mbed (the framework's names cannot come from a sys_darwin.xo
   call, gotchas/cabi-tls-exports-cannot-name-frameworks.md), or give
   it to Xo through decision 0232's sys_c.xo weak C calls.
2. `rthttp.tls_roots_system` (http.xo through tlsio.roots_system, and
   tlsio's check of which roots a client has): the C's sys_roots record,
   used by the Xo path only as the sentinel of "the system's roots"
   (since part 16 the Xo path parses them itself, static word 31). Next
   step: an Xo record (an rt.Any with a system flag) from
   tlsio.roots_system.
3. The build: selfhost/xo/tls.xo's needs_tls (and internal/xotool's
   tls.go, the Go driver's twin) compiles `-DXO_TLS` and links
   libxotls.a for every program calling the std/http client or server.
   After 1 and 2 it should do so only when `XO_LLRT_C` names tlsio or
   tlssrv; XO_NOLIBC could then allow TLS programs
   (selfhost/xo/main.xo:722-727 refuses them for Mbed's libc needs).
   That is a change under selfhost/, so it passes the fixed point.

After those three, the default path uses no Mbed TLS and no
fiat-crypto (fiat-crypto is compiled into libxotls.a only, through
third_party/mbedtls/xo/xo_p256.c).

**On the opt-in paths.** `XO_LLRT_C=tlsio` (tlsio_c.xo: the client on
xo_tls.c's tls_client_start, tls_read and tls_write) and
`XO_LLRT_C=tlssrv` (tlssrv_c.xo: the server's configuration,
handshake, reads, ALPN, close_notify) use Mbed TLS for everything.
xo_tls.c reaches back into Xo through cabi_tls.xo's exports
(tlscert.xo's cert_check, roots_pem, roots_system, expiry_msg,
apple_verify on Mbed's chains; tlsname.xo's host checks and texts).

**What the deletion removes** (one commit, after the owner's go):

- third_party/mbedtls (8.8 MB: mbedtls.tgz, src/, xo/ with
  xo_tls_config.h, xo_crypto_config.h, xo_p256.c, xo_tls_accel.c,
  win/pthread.h; mbedtls.go, gen.go, mbedtls_test.go) and
  third_party/fiat-crypto (180 KB: src/, fiatcrypto.go,
  fiatcrypto_test.go); scripts/vendor-mbedtls.sh and
  scripts/vendor-fiat-crypto.sh.
- std/llrt/xo_tls.c (1,091 lines) and its include and forwarders in
  std/llrt/xo_http.c (the TLS declarations, xo_tls_c_* and xo_cl_tls_*,
  `#include "xo_tls.c"`); xo_win.c's note on Mbed's pthread subset.
- std/native/llrt/tlsio_c.xo, tlssrv_c.xo, cabi_tls.xo; tlscert.xo's
  Mbed parts (cert_check, roots_pem, roots_system and its loaders'
  Mbed sinks, parse_der, x509_time, expiry_msg, apple_verify, host_bad;
  apple_trust, system_ders, pem_certs, rfc3339 stay); rthttp.xo's TLS
  declarations (cl_tls_*, tls_c_*, TCert, tls_cert, tls_roots,
  tls_roots_err, srv_tls; Roots stays as the Xo record).
- The build glue: selfhost/xo/tls.xo (361 lines), its uses in
  selfhost/xo/tools.xo and the needs_tls path, internal/xotool/tls.go
  (346 lines) and the TLS cache key in scripts/selfhost-keys.sh; the
  archive cache <user cache>/xo/mbedtls/<version>-<key>/ and
  $XO_TLS_CACHE; the embedded sources in the xo binary.
- Tests and docs: TestTLSDiff's and http_tls's runs under
  XO_LLRT_C=tlsio,tlssrv; decision 0105 superseded by 0202's
  Consequences; knowledgebase/infrastructure/toolchain.md's Mbed TLS
  section; this section of the audit.

**Before the deletion**: the default path at zero Mbed (1 to 3 above),
then a full milestone run on the default path, then the speed review.

## Test programs in std/llrt/testdata (test infrastructure, 3,579 lines)

Each includes the runtime with `XO_RT_NO_MAIN` and plays generated code;
run by internal/llvm tests through internal/xotool CompileC:

| File | Lines | Tests | Run by |
|---|---|---|---|
| alloc_threads.c | 327 | the heap in Xo (alloc.xo through its exports) under ThreadSanitizer and -O2 | TestRuntimeAlloc (threads_test.go:231) |
| crypto_diff.c | 153 | std/crypto's C on inputs from stdin against Go's crypto | TestCryptoDiff (crypto_diff_test.go:37) |
| crypto_ref.c | 804 | the hashes decision 0198 deleted from xo_crypto.c, kept as test data, included by crypto_diff.c | (with crypto_diff.c) |
| evloop.c | 363 | event loop and fibers: timers, socket echo, cancellation, select after, a child's pipe; both sched paths | TestRuntimeEventLoop (threads_test.go:295) |
| fault_resume.c | 50 | a fault unwinding through a C cleanup that calls _Unwind_Resume | TestRuntimeFaultResume (fault_resume_test.go:21) |
| http_diff.c | 205 | deleted 2026-10-15 (B28 follow up, xo-4b): TestHTTPParseDiff and FuzzHTTPDiff now send their inputs to the Xo server program http1DiffProgram built with the native backends and compare with net/http in process | TestHTTPParseDiff, FuzzHTTPDiff (http_diff_test.go) |
| http_fuzz.c, http_ref.c | 395, 1,123 | deleted 2026-10-15: TestHTTPFuzz mutation fuzzes the Xo parsers through built programs (the HTTP/1.1 server over TCP, HTTP/2 frames and HPACK over TLS, the client's response reading, std/json) with XO_LEAKCHECK=1; it found a middleware fault stopping the native server (fixed) | TestHTTPFuzz (http_diff_test.go) |
| rc_threads.c | 547 | shared reference counts between threads (the only caller of xo_rc_inc, xo_rc_dec, xo_rc_claim at xo_rt.c 498-505) | TestRuntimeThreads (threads_test.go:200) |
| realtime.c | 185 | real-time sections (deferred frees) | TestRuntimeRealtime (realtime_test.go:100) |
| tasks.c | 550 | the task runtime: spawn, channels, mutexes, cancellation; both sched paths | TestRuntimeTasks (threads_test.go:262) |

They go with the C they test: crypto_diff.c and crypto_ref.c with B08
and B09; rc_threads.c, alloc_threads.c, realtime.c with B13 and B21;
tasks.c and evloop.c with B17 to B22 (a test in Xo of the same races,
under ThreadSanitizer once Xo code can be instrumented); fault_resume.c
with B05; http_diff.c and http_fuzz.c with B10, B11, B28, B29. Other
test only C: internal/llvm/testdata/errno_probe.c (19, TestErrnoTables,
sysio_test.go:85), internal/cbind/testdata/tiny (127) and
internal/selfhost/testdata/ssa/stage6c and stage6c_skipped tiny.c and
tiny.h (254): `use c` fixtures, (c) by design.

## What still compiles C for the runtime

- **xo1 (selfhost/xo), every native build**: link_inputs copies every
  std/llrt .c into the build directory (selfhost/xo/main.xo:671-686) and
  runtime_object compiles xo_rt.c with cc or clang, cached by flags
  (selfhost/xo/link.xo:114, selfhost/xo/rt.xo:59-112), unless a prebuilt
  pack matches (rt.xo:187-470, decision 0157).
- **Zone data as C**: with cc as the linker, tz_source writes
  `xo_tzdata.c` (selfhost/xo/tz.xo:336-350); with the self hosted linker
  tz_object writes assembly (selfhost/xo/ld.xo:124-190,
  selfhost/xo/main.xo:698). Writing the assembly object always removes
  this C (a small follow up beside W0).
- **Mbed TLS**: selfhost/xo/tls.xo builds libxotls.a per C compiler
  (d).
- **XO_NOLIBC arm64**: nolibc_atomics writes assembly, not C
  (selfhost/xo/link.xo:172-193).
- **Packs**: `xo rtpack` (selfhost/xo/rtpack.xo; selfhost/xo/main.xo:89-91)
  and scripts/release.sh:148-158 (zig cc for linux/arm64) compile the
  runtime for releases.
- **Tests only**: internal/xotool/cc.go CompileC (cc.go:195-202) and
  IOFlags (cc.go:21-141, a mirror of link.xo's io_flags), and
  internal/xotool/tls.go (the Mbed TLS archive for sanitizer builds of
  the network goldens, tls.go:23-32), used by internal/llvm's tests.
- Nothing else under internal/ or scripts/ compiles runtime C; the
  scripts' `clang --version` lines only print the toolchain.

## Order of the remaining work (agent sized, a few hours each)

Ranked by dependencies first, then C retired per hour. "Now" items need
nothing new.

| # | Item | Retires | Needs first | First step |
|---|---|---|---|---|
| W0 | Dead C and stale switch plumbing | 144 C lines, plus the stale -D entries and test flags | nothing | **Done 2026-10-14** (see "W0 done" below) |
| W1 | (a) sched path | 1,285 | the next milestone run (Linux full tier and Windows batch on a main with sched stage 2) | **Done 2026-10-14** (see "W1 done" below): 1,284 lines of xo_rt.c |
| 1 | B01 clocks (done 2026-10-14, Windows measured: Clock.now 0.928 to 0.952 of the C path's cycles) | 132 | nothing | Windows: route sysio.clock_now's Windows arm and measure on LLVM (`WORKLOADS=clock bench/perf/sysio/measure.sh` in the Windows batch); darwin: choose the layer once into a word (as the vDSO address in `rt.sys_words` word 0) so clock_now has no `match target` |
| 2 | B02 globals and thread locals (rt.static_word and std/log done 2026-10-14; xo_sys.c after the next seed) | 188 | nothing | `rt.static_word(k)` in std/native/rt.xo and the three self hosted emitters (decision 0207's design), then xo_sys.c's helpers |
| 3 | B03 stdio, exit, env, args (done 2026-10-14) | 343 | 2 | after the next seed: delete the seed forwards and xo_rt.c's and xo_bytes.c's env C |
| 4 | B08 SHA-512 and rand | 258 | nothing (rand: a tw_rand_u64 export) | **Done 2026-10-14** (see "B08 done" below): SHA-512 deleted; the C Rand deleted once seed 0.10.0-23ce8ce2 had B08 (2026-10-14) |
| 5 | B10 HTTP/1.1 client parser (two items) | 1,558 | nothing | **Done 2026-10-14** (http-client-2026-10-14.md): client_do and parse_response in http1.xo, h2_request's URL from h2_target; 1,629 runtime C lines deleted, 265 added; bar 0.953 to 1.008 |
| 6 | B11 client HPACK | 583 | 5's hand off pattern | hpack.xo encode one field; the client's decode through hpack.xo |
| 7 | B12 response writer | 537 | nothing | **Done 2026-10-14** (http-writer-2026-10-14.md): http1w.xo through cabi_http.xo; the C wrappers (112 lines) go with B10's and the connection code's ports |
| 8 | B13 string and list objects (two items; stage 1 done 2026-10-14) | 816 | 2 | stage 1 done (string-list-objects-2026-10-14.md: xo_str_find deleted, xo_str_rune_count a seed shim until a seed has B13); stage 2 lists done (decision 0226: 0.883 to 1.021 of the C, arm64 and LLVM); next the string objects the same way, and deleting the C list entries once a seed emits the direct calls and LLVM calls the exports |
| 9 | Duration text bar, then the (a) text path (**done 2026-10-14**, decision 0227: bar accepted by the owner, the C path deleted) | 533 | nothing | decision 0194's ranked list on selfhost/arm64 (1.112 now); or ask the owner to accept LLVM's 0.925 with arm64's 1.112 |
| 10 | B04 xo_nolibc.c shrink (two items) | 1,463 | 3 (printf, FILE), 6 for the rest | malloc to alloc.xo exports, strerror and files to linux.xo; mem* as plain-name exports |
| 11 | B05 unwinder (three items; stage 1 done 2026-10-14, XO_NOLIBC linux/arm64, decision 0221) | 185 | nothing (XO_NOLIBC Linux first) | .eh_frame_hdr lookup and a CFI interpreter in unwind.xo; a resume intrinsic; then the personality routine as an export; the 12 nolibc fault goldens are the test |
| 12 | B06 faults in Xo | 319 | 3, 11 | the fault record and fault_exit; then Windows RtlVirtualUnwind through sys_windows.xo |
| 13 | B14 test world part 1 (two items) | 1,059 | 2 | testworld.xo records and the generator draws (gen.xo already calls them) |
| 14 | B16 Fs operations (two items) | 1,383 | 2 | fsops.xo POSIX over linux.xo and darwin.xo; then the Windows layer and xo_win.c 542-1029; the `stat$INODE64` name for darwin/amd64 |
| 15 | B27 zone data host side | 512 | 2 | a data symbol intrinsic (or xo1 emits the zone table as Xo); GetDynamicTimeZoneInformation; one Windows run |
| 16 | B21 heap platform calls | 389 | 2 | heap_chunk and heap_large over mmap and VirtualAlloc from alloc.xo |
| 17 | B17 contexts, budgets, shields | 454 | 1, 3 | ctx.xo with the context record; budgets over the monotonic clock |
| 18 | B18 wait lists, await, task record (two items) | 588 | 17 | one wait entry per context (decision 0212 stage 3), then await |
| 19 | B19 channels, select, Mutex[T] (two items) | 775 | 18 | chan.xo over the per context wait entry |
| 20 | B20 pool and fiber remnants | 707 | 17 to 19 | pool_init, root_worker, deadlock check, fibers_setup in pool.xo |
| 21 | B22 poller (two items) | 920 | 18, ntdll declarations | timers and descriptors in poll.xo over epoll and kqueue; then AFD and IOCP |
| 22 | B23 sockets and net API (two items) | 1,304 | 21, ws2_32 declarations | darwin socket layer in darwin.xo; then Winsock; then listen and dial in netio.xo |
| 23 | B24 stdin.xo | 387 | 21 | stdin over poller waits |
| 24 | B25 processes (three items) | 1,764 | 18, 21 | Windows CreateProcessW first (no new primitive), then darwin posix_spawn, then the Linux raw spawn |
| 25 | B26 signals, shutdown, entry (two items) | 475 | 19, 21 | C ABI callback entries in the emitters; then main and `_start` emitted |
| 26 | B15 test world part 2 | 494 | 18, 21, 25 | the virtual clock over poll.xo's timers |
| 27 | B28 HTTP server (four items) | 3,278 | 7 for h2_head; 18, 21, 22 for the loops | the HTTP/2 frame reader and writer (bytes only) first |
| 28 | B29 HTTP client (two items) | 1,288 | 5, 6, 21, 22 | the client loop over netio.xo |
| 29 | B09 constant time, Ed25519 (two items) | 505 | the prerequisite 5 ADR | value barrier and constant time select in the emitters with a branch check; crypto.equal; ed25519.xo |
| 30 | B30 TLS pre-work (two items) | 573 | 2 | Linux root files in roots.xo; macOS Security.framework and Windows crypt32 declarations |
| 31 | B31 TLS in Xo and the third party | 813 plus 200,915 | 29, 30, 27, 28 | decision 0202 |
| last | BZ | 628 | everything above | goes with the last C caller |

Without TLS (B31 and the third party) and BZ, the list is 23,237 lines
of std/llrt in (b) plus the 2,060 of (a). Items 1 to 9 (11 agent
items) need no new runtime primitive except `rt.static_word`; items 10
to 12 need the unwinder's resume intrinsic; 17 to 26 need decision 0212
stage 3's per context wait entry, then ntdll and ws2_32 declarations and
C ABI callback entries.

### (a) items that can be deleted after the next milestone platform run

- **sched** (xo_rt.c, 1,285 lines in the 22 ranges above, plus
  sched_c.xo, `rt.sched_c`, selfhost/xo/link.xo:395-397,
  internal/xotool/cc.go:137-138, the second flag set at
  internal/llvm/threads_test.go:268, 299): needs a clean Linux full tier
  and the Windows batch on a main that contains decision 0212 stage 2.
- **net** (std/native/llrt/net_c.xo and selfhost/xo/main.xo:728-735):
  the Linux full tier for decision 0206 stage 3; the C lines it guards
  stay with B23.
- Not run gated: text (the Duration text bar), the on demand intrinsics
  (an SSA check), and the dead groups (W0, now).
