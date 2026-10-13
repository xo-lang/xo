# The C that Xo compiles: audit by prerequisite (decision 0195, first step)

Date: 2026-10-13. Main at 3a13792a (decision 0193 step 5 merged). An
analysis: no runs, no timings. Line counts are `wc -l` of the files (raw
lines, comments and blank lines included); group boundaries are at
function starts, so a group carries the comment above its first
function. The tables were made by a script over the line ranges listed
below (kept out of the repo per the repro script rule); every range was
checked to cover its file with no gap.

## Totals

| What | Lines |
|---|---|
| std/llrt (15 .c files, no headers) | 30,669 |
| third_party/mbedtls/src .c (raw; the Xo configuration compiles out much of it) | 120,810 |
| third_party/mbedtls/src .h | 74,707 |
| third_party/mbedtls/xo (xo_p256.c 810, xo_tls_accel.c 153, configs and Windows pthread.h 209) | 1,172 |
| third_party/fiat-crypto/src (p256_64.c, p256_scalar_64.c) | 4,226 |
| **All C that native builds compile** | **231,584** |
| of which .c files only | 156,668 |

What native builds compile, from internal/build and selfhost/xo: the
runtime is one translation unit, `std/llrt/xo_rt.c`, which includes
xo_win.c (Windows) and xo_std.c, which includes every other std/llrt
file (xo_http.c includes xo_h2.c and xo_tls.c). Programs that use
std/http's TLS also compile Mbed TLS and fiat-crypto (internal/build
tls.go, selfhost/xo/tls.xo: `SourceDirs` of third_party/mbedtls). Not
counted above, and not runtime:

- `xo_tzdata.c`: generated zone data, written only by the Go driver
  (internal/build tzdata.go); xo1 already emits it as an assembly object
  (selfhost/xo/ld.xo `tz_object`), so it leaves with the Go driver.
- `cffi/xo_<prefix>.c`: generated adapters for a program's own `use c`
  bindings (internal/build cffi.go, selfhost/xo/cffi.xo), and the C files
  such a program links (examples/realtime-control/rtclock.c, 21 lines).
  User C, out of the runtime goal (see owner decision D).
- Test only C: std/llrt/testdata 2,416 lines (rc_threads, tasks, evloop,
  realtime, fault_resume, crypto_diff, json_diff, http_diff, http_fuzz),
  internal/build/testdata/tls_accel.c 395, internal/cbind and
  internal/selfhost stage6c tiny.c and tiny.h 381. They test C that this
  plan removes and go with it.
- std/rt's curg_amd64.s and curg_arm64.s (14 lines) belong to the Go
  backend's runtime, which is Go, not C.
- The wasm backend has its own WAT runtime and compiles no C.

## Legend

The prerequisites (decision 0195's four, split as the task asked):

- (0) nothing: portable now under decision 0193's bar. `(0) held by C
  callers`: portable, but other C calls it, and decision 0180 item 3
  (C never calls compiled Xo) keeps a C copy until those callers move.
- (1) atomics and memory ordering.
- (2) raw memory: the allocator, reference counts, object and element
  layout, intrusive structures.
- (3) raw system calls and platform library calls (Linux raw syscalls,
  macOS libSystem, Windows kernel32, ntdll, ws2_32), including C ABI
  entry points the platform calls back (thread start, signal handlers,
  the unwinder's personality routine).
- (4) SIMD or word at a time scans.
- (5) constant time code.
- (6) the per CPU task switch (assembly) and thread local state.
- P: in progress, decision 0193 step 6 (agent port6: xo_http.c and
  xo_h2.c parsing and framing, xo_fs.c path walks, xo_proc.c argument
  building). Not edited here. Where port6 draws the line inside xo_fs.c
  and xo_proc.c is estimated.
- G: a C path kept behind `XO_LLRT_C` for a piece already ported (not
  in any pack; compiled only when selected).

"Unblocked by" is the last prerequisite the group needs in the
recommended order below (0, 3, 1, 2, 6, 4, 5), so each line counts once.

## Counts


### By prerequisite (std/llrt, 30,669 lines)

| Bucket | Lines | Notes |
|---|---|---|
| P, in progress (decision 0193 step 6) | 4,567 | xo_http.c 2,267, xo_h2.c 1,896, xo_proc.c 259, xo_fs.c 145 |
| G, C paths kept behind XO_LLRT_C | 4,654 | 4,313 deletable after a full tier; text's 341 stay (Duration text). Decision 0198 (2026-10-13) deleted unicode, jsontext, num, strings, and the shared 82: 3,354 of these lines; tz, report, show (959) wait for a full tier of step 5 |
| (0) portable now | 1,799 | sized integer operations, crypto hashes, glob, generators, str_hash |
| (0) held by C callers | 1,806 | fmt_int/fmt_go_g, fault texts, UTF-8 helpers, address policy, TLS name checks |
| (3) system and platform calls | 3,105 | xo_win.c 1,188 (Windows only), zone files, fs, proc spawn |
| (1) atomics | 4,966 | locks, parks, budgets, stdin, proc and fs handles, sockets, signals |
| (2) raw memory | 6,574 | strings and lists, RC, scheduler, channels, event loop, http server loop |
| (6) task switch and thread state | 1,166 | fiber switch, fiber stacks, ctx, allocator heap pointer, entry |
| (4) SIMD / word scans | 313 | str_find/eq/slice, split, lines, rune_count, utf8_valid, to_hex |
| (5) constant time | 1,719 | Ed25519, crypto.equal, xo_tls.c over Mbed TLS (plus 509 TLS policy lines held by its C callers) |

### Lines that need each prerequisite at all (std/llrt, outside P and G)

A group often needs several; this counts a group under each one it needs.

| Prerequisite | Lines that need it |
|---|---|
| (3) system and platform calls | 11,239 |
| (1) atomics | 11,108 |
| (2) raw memory | 7,398 |
| (5) constant time | 1,719 |
| (6) task switch and thread state | 1,166 |
| (4) SIMD / word scans | 313 |

### Per file (lines by what unblocks them)

`0*` is (0) held by C callers. The third party C (200,915 lines) is all
(5): see "Third party" below.

| File | Lines | P | G | 0* | 0 | 3 | 1 | 2 | 6 | 4 | 5 |
|---|---|---|---|---|---|---|---|---|---|---|---|
| std/llrt/xo_rt.c | 7419 | 0 | 0 | 413 | 68 | 197 | 1769 | 3742 | 1166 | 64 | 0 |
| std/llrt/xo_std.c | 1415 | 0 | 747 | 399 | 47 | 21 | 0 | 0 | 0 | 201 | 0 |
| std/llrt/xo_bytes.c | 865 | 0 | 219 | 16 | 295 | 116 | 31 | 140 | 0 | 48 | 0 |
| std/llrt/xo_testing.c | 1667 | 0 | 54 | 336 | 518 | 60 | 452 | 247 | 0 | 0 | 0 |
| std/llrt/xo_time.c | 1435 | 0 | 868 | 57 | 0 | 415 | 95 | 0 | 0 | 0 | 0 |
| std/llrt/xo_stdin.c | 435 | 0 | 0 | 0 | 0 | 0 | 435 | 0 | 0 | 0 | 0 |
| std/llrt/xo_crypto.c | 1544 | 0 | 0 | 0 | 867 | 128 | 0 | 0 | 0 | 0 | 549 |
| std/llrt/xo_tls.c | 1679 | 0 | 0 | 509 | 0 | 0 | 0 | 0 | 0 | 0 | 1170 |
| std/llrt/xo_win.c | 1188 | 0 | 0 | 0 | 0 | 1188 | 0 | 0 | 0 | 0 | 0 |
| std/llrt/xo_proc.c | 2099 | 259 | 0 | 76 | 0 | 761 | 1003 | 0 | 0 | 0 | 0 |
| std/llrt/xo_fs.c | 875 | 145 | 0 | 0 | 0 | 219 | 511 | 0 | 0 | 0 | 0 |
| std/llrt/xo_http.c | 3927 | 2267 | 0 | 0 | 4 | 0 | 670 | 986 | 0 | 0 | 0 |
| std/llrt/xo_h2.c | 3355 | 1896 | 0 | 0 | 0 | 0 | 0 | 1459 | 0 | 0 | 0 |
| std/llrt/xo_unicode.c | 1783 | 0 | 1783 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| std/llrt/xo_json.c | 983 | 0 | 983 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 |
| **total std/llrt** | 30669 | 4567 | 4654 | 1806 | 1799 | 3105 | 4966 | 6574 | 1166 | 313 | 1719 |


## Groups, file by file

### std/llrt/xo_rt.c (7419 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-65 | 65 | header, platform includes | (3) | (3) |  |
| 66-476 | 411 | allocator: size classes, per thread heaps, central lists | (1), (2), (6) | (6) | heap pointer is __thread; magazines under a mutex |
| 477-586 | 110 | reference counts (thread local and shared) | (1), (2) | (2) |  |
| 587-623 | 37 | test world state (virtual clock words) | (1) | (1) |  |
| 624-744 | 121 | buffered stdout/stderr | (3), (1) | (1) |  |
| 745-867 | 123 | fmt_int, fmt_go_g (Int and Float text) | (0) held by C callers | (0) | num.xo has the Xo version; C copy held by C callers (fault texts) |
| 868-942 | 75 | fault messages (xo_fault_*) | (0) held by C callers | (0) | called by C too |
| 943-958 | 16 | str_eq, str_cmp | (4) | (4) | memcmp |
| 959-1144 | 186 | string and list objects: new, append, share, free | (2) | (2) |  |
| 1145-1293 | 149 | task, park, context types; handles; stats | (1), (2) | (2) |  |
| 1294-1379 | 86 | real-time sections (deferred frees) | (2) | (2) |  |
| 1380-1485 | 106 | spin hint, futex wait/wake, locks | (3), (1) | (1) | Linux futex syscall; macOS __ulock_wait/wake (libSystem); Windows WaitOnAddress |
| 1486-1693 | 208 | mutex policy, parks, wait nodes | (3), (1) | (1) | pthread cond on POSIX, SRW/condvar on Windows |
| 1694-1716 | 23 | ctx: the current context (thread or fiber local) | (6) | (6) |  |
| 1717-2106 | 390 | fault unwinding: personality, LSDA, raise | (3), (2) | (2) | _Unwind_* (libunwind/libgcc), SEH on Windows; personality is a C ABI callback |
| 2107-2393 | 287 | resource budgets | (3), (1) | (1) |  |
| 2394-3048 | 655 | worker pool: run queues, steal, inject, deadlock check | (3), (1), (2) | (2) |  |
| 3049-3337 | 289 | scopes and tasks | (1), (2) | (2) |  |
| 3338-3571 | 234 | fiber record and xo_fiber_switch / xo_fiber_start (assembly per CPU and OS) | (6) | (6) |  |
| 3572-3935 | 364 | sanitizer hooks, fiber stacks (mmap, guard page), park, worker main | (3), (2), (6) | (6) |  |
| 3936-4184 | 249 | task spawn, done, await, scope close | (1), (2) | (2) |  |
| 4185-4682 | 498 | channels and select | (1), (2) | (2) |  |
| 4683-4880 | 198 | Mutex[T] | (1), (2) | (2) |  |
| 4881-4937 | 57 | Atomic[T] | (1) | (1) |  |
| 4938-5869 | 932 | event loop: timers, poller (kqueue, epoll, AFD/IOCP) | (3), (1), (2) | (2) | Linux epoll/eventfd/pipe2; macOS kqueue (libSystem); Windows AFD via ntdll + IOCP |
| 5870-6041 | 172 | clock, sleeps, deadlines, budget open/end | (3), (1) | (1) |  |
| 6042-6116 | 75 | network status and messages | (3) | (3) |  |
| 6117-6331 | 215 | host:port split, address admission policy | (0) held by C callers | (0) | called by the C listen/dial |
| 6332-6897 | 566 | sockets: listen, accept, dial (getaddrinfo), read, write, Unix sockets | (3), (1) | (1) | getaddrinfo is libc on Linux (see owner decision B) |
| 6898-6945 | 48 | utf8_valid | (4) | (4) |  |
| 6946-7013 | 68 | str_hash, str_quote, floor_div | (0) | (0) |  |
| 7014-7070 | 57 | environment | (3) | (3) |  |
| 7071-7285 | 215 | signals and two phase shutdown | (3), (1) | (1) | signal handler is a C ABI callback |
| 7286-7419 | 134 | entry: main, 1 GiB main thread, Windows init | (3), (6) | (6) |  |

### std/llrt/xo_std.c (1415 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-10 | 10 | header | (0) | (0) |  |
| 11-13 | 3 | unicode C path include | C path kept | unicode |  |
| 14-177 | 164 | nstr, nbuf, UTF-8 decode/encode, is_space | (0) held by C callers | (0) | used by other C |
| 178-285 | 108 | case mapping C path | C path kept | unicode |  |
| 286-355 | 70 | mem_index, starts/ends_with, find, slice, byte | (4) | (4) | memchr/memcmp/copy |
| 356-395 | 40 | trim C path | C path kept | strings |  |
| 396-477 | 82 | rune_count, split, lines | (4) | (4) | lost the bar (1.09x to 1.13x) |
| 478-511 | 34 | replace C path | C path kept | strings |  |
| 512-560 | 49 | repeat, join, rs_append | (2), (4) | (4) |  |
| 561-589 | 29 | n_parse_int | (0) held by C callers | (0) | xo_http.c, xo_json.c callers |
| 590-603 | 14 | Int.parse C path | C path kept | text |  |
| 604-685 | 82 | n_lower_eq, n_parse_float | C path kept | num+jsontext | unguarded but only the C paths of num and jsontext call it |
| 686-700 | 15 | Float.parse C path | C path kept | num |  |
| 701-827 | 127 | n_fmt_duration | (0) held by C callers | (0) | xo_rt.c budget text calls it |
| 828-839 | 12 | Duration text C path | C path kept | text | Duration text lost the bar: 1.171x arm64, 1.087x LLVM |
| 840-886 | 47 | n_fmt_time | (0) held by C callers | (0) | xo_http.c log time |
| 887-952 | 66 | Time text, Debug quoting C path | C path kept | text |  |
| 953-973 | 21 | xo_exit | (3) | (3) |  |
| 974-1000 | 27 | std/json base64 C path | C path kept | jsontext |  |
| 1001-1023 | 23 | b64 encode C path | C path kept | num |  |
| 1024-1043 | 20 | b64_val | (0) held by C callers | (0) | n_b64_decode (num C path) and xo_crypto.c base64 call it |
| 1044-1111 | 68 | b64 decode, hex C path | C path kept | num |  |
| 1112-1264 | 153 | Duration.parse C path | C path kept | text |  |
| 1265-1276 | 12 | n_days_from_civil | (0) held by C callers | (0) |  |
| 1277-1372 | 96 | Time parse C path | C path kept | text |  |
| 1373-1377 | 5 | blank | (0) | (0) |  |
| 1378-1383 | 6 | xo_json.c include | C path kept | jsontext |  |
| 1384-1415 | 32 | includes of the other files | (0) | (0) |  |

### std/llrt/xo_bytes.c (865 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-200 | 200 | sized integer operations (int_op, int_conv, rune_of), f32 | (0) | (0) | decision 0193 step 5 next (b) |
| 201-209 | 9 | UInt64 text C path | C path kept | show |  |
| 210-222 | 13 | rune validity | (0) | (0) |  |
| 223-250 | 28 | rune text C path | C path kept | show |  |
| 251-350 | 100 | rs_push_byte/word/words, rs_append_rune, rs_append_bytes | (2) | (2) |  |
| 351-366 | 16 | bytes_span | (4) | (4) |  |
| 367-406 | 40 | rs_with_cap, slice checks | (2) | (2) |  |
| 407-559 | 153 | Rune classes and quoting C path | C path kept | unicode |  |
| 560-575 | 16 | bytes_decode, str_rune_count | (4) | (4) | rune_count lost the bar (1.13x arm64) |
| 576-591 | 16 | Bytes.to_hex | (4) | (4) | lost the bar (1.09x arm64) |
| 592-664 | 73 | C versions of the inline intrinsics (int_wrap, u64_mul_hi, float bits, bytes_word, int_fits) | (0) | (0) | deletable once every emitter lowers them |
| 665-693 | 29 | hex decode C path | C path kept | num |  |
| 694-702 | 9 | helpers | (0) | (0) |  |
| 703-818 | 116 | fs_read, args, program name | (3) | (3) |  |
| 819-834 | 16 | fault_bytes_index | (0) held by C callers | (0) |  |
| 835-865 | 31 | const_once, consts_free | (1) | (1) |  |

### std/llrt/xo_testing.c (1667 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-89 | 89 | test world records | (1) | (1) |  |
| 90-149 | 60 | tw_init (environment) | (3) | (3) |  |
| 150-333 | 184 | test index, fail, exit, virtual clock kick and advance | (1) | (1) |  |
| 334-513 | 180 | random numbers, property generator draws | (0) | (0) |  |
| 514-567 | 54 | property report C path | C path kept | report |  |
| 568-711 | 144 | generator choices and values | (0) | (0) |  |
| 712-896 | 185 | test output, env, exit code, log records | (0) held by C callers | (0) |  |
| 897-1143 | 247 | in-memory listeners | (1), (2) | (2) |  |
| 1144-1294 | 151 | fakes, records, memfs | (0) held by C callers | (0) |  |
| 1295-1304 | 10 | virtual clock read | (1) | (1) |  |
| 1305-1498 | 194 | glob matching | (0) | (0) |  |
| 1499-1667 | 169 | cleanup, signals, test main | (3), (1) | (1) |  |

### std/llrt/xo_time.c (1435 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-46 | 46 | header | (3) | (3) |  |
| 47-741 | 695 | zone rules C path | C path kept | tz |  |
| 742-798 | 57 | zip directory reader | (0) held by C callers | (0) |  |
| 799-975 | 177 | zone files, readlink | (3) | (3) |  |
| 976-1147 | 172 | Windows zone registry, host data | (3) | (3) | Windows only |
| 1148-1242 | 95 | zone data and loaded zone table | (1) | (1) |  |
| 1243-1248 | 6 | tz_now | (3) | (3) |  |
| 1249-1393 | 145 | zone lookup C path | C path kept | tz |  |
| 1394-1407 | 14 | local zone name | (3) | (3) |  |
| 1408-1435 | 28 | offset and abbreviation C path | C path kept | tz |  |

### std/llrt/xo_stdin.c (435 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-81 | 81 | stdin queue | (1) | (1) |  |
| 82-435 | 354 | stdin reader thread, isatty, terminal width, echo | (3), (1) | (1) |  |

### std/llrt/xo_crypto.c (1544 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-21 | 21 | header | (3) | (3) |  |
| 22-128 | 107 | entropy (getrandom, getentropy, BCryptGenRandom), random numbers | (3) | (3) |  |
| 129-311 | 183 | SHA-256, HMAC, PBKDF2 | (0) | (0) | bar unmeasured; 32 bit rotates |
| 312-671 | 360 | BLAKE2b, Argon2 | (0) | (0) | bar unmeasured |
| 672-778 | 107 | base64, parameters, sha256/hmac API | (0) | (0) |  |
| 779-790 | 12 | crypto.equal | (5) | (5) |  |
| 791-893 | 103 | pbkdf2, argon2id API, parameters | (0) | (0) |  |
| 894-963 | 70 | verify_password (constant time compare) | (5) | (5) |  |
| 964-1077 | 114 | SHA-512 | (0) | (0) |  |
| 1078-1544 | 467 | Ed25519 (field, group, scalar, sign, verify) | (5) | (5) |  |

### std/llrt/xo_tls.c (1679 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-131 | 131 | TLS init, configuration | (5) | (5) |  |
| 132-291 | 160 | PEM decode, cert type list | (0) held by C callers | (0) |  |
| 292-456 | 165 | certificate load (mbedtls parse) | (5) | (5) |  |
| 457-488 | 32 | roots from PEM | (5) | (5) |  |
| 489-686 | 198 | system roots (files; Security.framework on macOS) | (3), (5) | (5) |  |
| 687-932 | 246 | BIO callbacks, connection read/write, ALPN | (3), (5) | (5) |  |
| 933-1121 | 189 | hostname and IP matching | (0) held by C callers | (0) |  |
| 1122-1249 | 128 | SNI callback, server, handshake | (5) | (5) |  |
| 1250-1351 | 102 | certificate time and hostname messages | (0) held by C callers | (0) |  |
| 1352-1482 | 131 | Apple trust verify, client verify | (3), (5) | (5) |  |
| 1483-1540 | 58 | alert text | (0) held by C callers | (0) |  |
| 1541-1679 | 139 | client start; stubs without XO_TLS | (5) | (5) |  |

### std/llrt/xo_win.c (1188 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-1188 | 1188 | POSIX subset on Win32 and Winsock (threads, clocks, files, dirs, env, entropy) | (3) | (3) | Windows only: kernel32, ntdll, ws2_32 |

### std/llrt/xo_proc.c (2099 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-594 | 594 | pipes, reap, PATH lookup, posix_spawn run | (3) | (3) |  |
| 595-805 | 211 | argument and environment building, command parse | in progress | decision 0193 step 6 | decision 0193 step 6 (port6) |
| 806-972 | 167 | command free, pipes, start | (3) | (3) |  |
| 973-1020 | 48 | Windows command line quoting | in progress | decision 0193 step 6 | decision 0193 step 6 (port6) |
| 1021-1391 | 371 | CreateProcess, wait, feed, collect, run | (3), (1) | (1) |  |
| 1392-1467 | 76 | fake processes | (0) held by C callers | (0) |  |
| 1468-2099 | 632 | exec, spawn, child I/O, kill | (3), (1) | (1) |  |

### std/llrt/xo_fs.c (875 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-193 | 193 | status, stat, names, write, rename, remove | (3) | (3) |  |
| 194-301 | 108 | remove_all, mkdir_all walks | in progress | decision 0193 step 6 | decision 0193 step 6 (port6), estimated split |
| 302-327 | 26 | chtimes | (3) | (3) |  |
| 328-364 | 37 | create_temp naming | in progress | decision 0193 step 6 | decision 0193 step 6 (port6), estimated split |
| 365-875 | 511 | cwd, file handles, copy, perm, symlink, locks | (3), (1) | (1) |  |

### std/llrt/xo_http.c (3927 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-1866 | 1866 | tables, request targets, request reading, response writing | in progress | decision 0193 step 6 | decision 0193 step 6 (port6) |
| 1867-2568 | 702 | server handles and connection loop | (3), (1), (2) | (2) |  |
| 2569-2811 | 243 | streamed bodies | (1), (2) | (2) |  |
| 2812-2852 | 41 | values of any type | (2) | (2) |  |
| 2853-3074 | 222 | text helpers of std/http | in progress | decision 0193 step 6 | decision 0193 step 6 (port6) |
| 3075-3140 | 66 | logging | (3), (1) | (1) |  |
| 3141-3282 | 142 | client connection I/O | (3), (1) | (1) |  |
| 3283-3461 | 179 | client response parse | in progress | decision 0193 step 6 | decision 0193 step 6 (port6) |
| 3462-3923 | 462 | client: pool, redirects, request | (3), (1) | (1) |  |
| 3924-3927 | 4 | includes | (0) | (0) |  |

### std/llrt/xo_h2.c (3355 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-1896 | 1896 | HPACK, frames, server frame handling | in progress | decision 0193 step 6 | decision 0193 step 6 (port6) |
| 1897-2672 | 776 | responses, sending, connection fiber | (1), (2) | (2) |  |
| 2673-3355 | 683 | client over h2 | (1), (2) | (2) |  |

### std/llrt/xo_unicode.c (1783 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-1783 | 1783 | Unicode tables and operations C path | C path kept | unicode | deleted (decision 0198, 2026-10-13) |

### std/llrt/xo_json.c (983 lines)

| Lines | Count | What | Needs | Unblocked by | Note |
|---|---|---|---|---|---|
| 1-983 | 983 | std/json parser C path | C path kept | jsontext | deleted (decision 0198, 2026-10-13) |

## C paths kept for pieces already ported (decision 0193 steps 1 to 5, decision 0180)

| Module | Switch | Lines | Where | Bar | Delete after a full tier? |
|---|---|---|---|---|---|
| unicode | `XO_UNICODE=c`, `-DXO_UNICODE_C` | 2,047 | xo_unicode.c 1,783, xo_std.c 111, xo_bytes.c 153 | holds on both (decisions 0184, 0192; 0180 item 7) | deleted (decision 0198, the ADR 0180 asks for) |
| jsontext | `XO_LLRT_C=jsontext` | 1,016 | xo_json.c 983, xo_std.c 33 | holds (step 4) | deleted (decision 0198; json_diff.c 115 and http_fuzz.c's JSON part 38 too) |
| tz | `XO_LLRT_C=tz` | 868 | xo_time.c | holds (step 5) | yes |
| num | `XO_LLRT_C=num` | 135 | xo_std.c 106, xo_bytes.c 29 | holds (step 3) | deleted (decision 0198) |
| num and jsontext | unguarded | 82 | xo_std.c 604-685 (n_lower_eq, n_parse_float) | | deleted (decision 0198; only their C paths called it) |
| strings | `XO_LLRT_C=strings` | 74 | xo_std.c | holds (step 2) | deleted (decision 0198) |
| report | `XO_LLRT_C=report` | 54 | xo_testing.c | holds (step 5) | yes |
| show | `XO_LLRT_C=show` | 37 | xo_bytes.c | holds (step 5) | yes |
| text | `XO_LLRT_C=text` | 341 | xo_std.c | Duration text misses it (1.171x arm64, 1.087x LLVM, decision 0194) | no: the module's one switch keeps all 341 until Duration text passes |
| **total** | | **4,654** | | | **4,313 deletable**, 341 kept; 3,354 deleted by decision 0198 (3,373 with the blank and comment lines around them), 959 left (tz, report, show) |

Deleting each also removes its `<name>_c.xo`, its IOFlags entry (and
xo1's `io_flags`), and the SSA builders' C branch (decision 0193 item 3).
The C versions of the inline intrinsics in xo_bytes.c 592-664 (73 lines:
`int_wrap`, `u64_mul_hi`, `float_bits`, `bytes_word`, `int_fits`) are a
fallback for "any other call"; they go once every emitter lowers them
inline (a grep of the emitted assembly over the goldens shows no call).

## Pieces that lost the bar, and what gives them a rematch

| Piece | Lines (C) | Measured | Rematch |
|---|---|---|---|
| Str.split, Str.lines | 82 with rune_count (xo_std.c 396-477) | split 1.09x, lines 1.13x on LLVM (decision 0193 item 2) | (4): an inline find byte (16 bytes a step) and (2)'s inline copy, so a piece costs no calls |
| Str.rune_count, Bytes rune count | xo_std.c above, xo_bytes.c 560-575 (16) | 1.13x on arm64 | (4): a popcount intrinsic over `rt.bytes_word` (8 bytes a word: count bytes that are not 10xxxxxx); possible before the rest of (4), since `bytes_word` exists |
| Bytes.to_hex | xo_bytes.c 576-591 (16) | 1.09x on arm64 (16 bytes a call) | (4) a table lookup in a vector register (NEON `tbl`), or decision 0193 step 3 next (d): a 32 byte push primitive, which is (2) light |
| Duration text | xo_std.c 828-839 (12) | 1.171x arm64, 1.087x LLVM | none of (1) to (6): decision 0194's ranked list (wide constants as logical immediates, ranges of call results for the checked shifts) |

## Recommended order, and what each step unblocks

Top down: the policy that calls C moves first, the leaves that all C
calls (allocator, reference counts) last, so decision 0180 item 3 (C
never calls compiled Xo) holds at every step and no C ABI export of an
Xo runtime function is needed except platform callbacks.

| Step | Prerequisite | std/llrt lines unblocked | Running total moved or deleted |
|---|---|---|---|
| now | delete the 4,313 lines of C paths that hold the bar (after a full tier) | 4,313 | 4,313 |
| now | decision 0193 step 6 (port6, in progress) | 4,567 | 8,880 |
| now | (0) portable now, under 0193's rules | 1,799 | 10,679 |
| with their callers | (0) held by C callers | 1,806 | moves with the step that moves the callers |
| 1 | (3) system and platform calls, C ABI callbacks | 3,105 | 13,784 |
| 2 | (1) atomics | 4,966 | 18,750 |
| 3 | (2) raw memory | 6,574 | 25,324 |
| 4 | (6) task switch and thread state | 1,166 | 26,490 |
| 5 | (4) SIMD and word scans (independent: can run in parallel with any step) | 313 | 26,803 |
| 6 | (5) constant time | 1,719 in std/llrt, 509 TLS policy, 200,915 third party | 28,522 (30,328 with the held rows: all but text's 341) |

Running totals exclude the (0) held by C callers row; each such group
moves in the step that moves its last C caller, so the final total with
it is 30,328 plus Duration text's C path once it passes. Why this order:
(3) first because it needs no change to the memory model (system call
structures marshal through Bytes) and everything else needs it; (1)
before (2) because 4,966 lines need atomics and system calls but no raw
memory (locks, parks, handles, budgets), against 843 lines that would
need raw memory but not atomics; (6) after both because the scheduler's
fiber code is all three; (4) is small and independent; (5) is last, as
decision 0195 already says.

## Design sketches (internal intrinsics only)

Every intrinsic below is declared in `std/native/rt.xo` (or a sibling
declaration file such as `native/sys_<os>.xo`), visible only to the
implementation modules that `use native/rt` (decision 0074): no user
visible language change. Lowering order per the Go freeze: selfhost/arm64,
selfhost/llvm, selfhost/wasm first; internal/arm64 and internal/llvm
mirror them while the Go driver remains (as step 3's intrinsics did).
Operand kinds that must be constants (orders, widths) are checked by the
SSA builder, like `int_wrap`'s op.

### (3) System and platform calls (unblocks 3,105; needed by 8,134 more)

- `rt.sys6(nr, a0, ..., a5) -> Int` (Linux): `svc #0` with x8 = nr on
  arm64, `syscall` on x86_64; returns the raw result (minus errno).
  selfhost/arm64 and internal/arm64 inline it; the LLVM emitters as an
  inline asm call. The Linux `rt_sigaction` restorer is a two
  instruction trampoline the emitter adds with the first use.
- Platform library functions: a declaration file per OS
  (`native/sys_darwin.xo`, `sys_windows.xo`) whose bodiless `fn`s lower
  to calls of the bare C symbol (no `xo_` prefix) with the C ABI: Int as
  a word, Float as a double, `Bytes` as its data address. Needed in the
  emitters: Apple arm64 variadic calls (`open`, `fcntl`, `ioctl` pass
  variadic arguments on the stack), and on Windows the dllimport of
  kernel32, ntdll, ws2_32, bcrypt, advapi32 (LLVM only today, decision
  0197). errno: `__error()` on macOS, the negative result on Linux,
  `GetLastError`/`WSAGetLastError` on Windows.
- `rt.bytes_addr(b) -> Int` and `rt.bytes_put_word(b, i, w)` (with the
  existing `bytes_word`): C structures (sockaddr, stat, kevent,
  epoll_event, OVERLAPPED) are Bytes buffers read and written by word.
- C ABI entry points: `rt.c_entry_addr(f) -> Int` for a top level
  function of a native module, which the emitter gives a C ABI prologue
  (callee saved registers, no Xo context assumed): thread start
  routines, signal handlers, the unwinder's personality routine, the
  Windows console handler. qsort and pthread_once uses go (Xo sorts and
  an atomic once).
- wasm: nothing; its runtime's I/O is host imports in WAT.
- Per OS interface: Linux, raw syscalls (epoll, eventfd2, pipe2, futex,
  mmap, openat, getdents64, statx, getrandom, rt_sigaction, sockets,
  clone3 for threads) if owner decision B picks no libc; macOS,
  libSystem (kevent, `__ulock_wait`/`__ulock_wake`, pthread, mmap,
  posix_spawn, getentropy, sigaction, getaddrinfo, `_Unwind_*`,
  Security.framework for TLS roots); Windows, kernel32 (threads,
  WaitOnAddress, files, CreateProcessW, VirtualAlloc), ntdll (AFD
  polling, RtlVirtualUnwind), ws2_32, bcrypt, advapi32 (zone registry).
  xo_win.c (1,188) disappears: its POSIX shim exists only because the C
  runtime is POSIX shaped; the Xo runtime calls Win32 directly.

### (1) Atomics and memory ordering (unblocks 4,966)

- On a `List[Int]` slot (no raw memory needed): `rt.atomic_load(xs, i,
  ord)`, `rt.atomic_store(xs, i, v, ord)`, `rt.atomic_swap`,
  `rt.atomic_cas(xs, i, old, new, ord) -> Bool`, `rt.atomic_add(xs, i,
  d, ord) -> Int` (fetch add), `rt.fence(ord)`, `rt.spin_hint()`; ord a
  constant (relaxed, acquire, release, acq_rel, seq_cst). Unchecked index
  and no copy on write (like `bytes_word`): a list used as atomic cells
  is made at its final size and never grown; the existing shared count
  mechanism (decision 0058) covers handing it to other threads.
  `rt.list_slot_addr(xs, i) -> Int` gives the address futex,
  `__ulock_wait`, and WaitOnAddress need (a 32 bit futex word is the low
  half on little endian targets).
- Lowering: arm64 `ldar`, `stlr`, `swpal`, `casal`, `ldaddal` (LSE; every
  Apple arm64 CPU has it; Linux arm64 at ARMv8.0 needs `ldaxr`/`stlxr`
  loops or a feature check), `dmb ish`, `yield`; LLVM `load atomic`,
  `store atomic`, `atomicrmw`, `cmpxchg`, `fence`; wasm (one thread)
  plain loads and stores.
- SSA: the calls are memory effects the optimizer keeps in order (no
  CSE, no hoisting out of loops, no load forwarding across them), in
  internal/ssa and selfhost/ssa alike.

### (2) Raw memory (unblocks 6,574)

- `rt.mem_load(a, off, w)`, `rt.mem_store(a, off, v, w)` (w 1, 2, 4, 8,
  constant), `rt.mem_copy(dst, src, n)`, `rt.mem_move`, `rt.mem_set`,
  `rt.mem_cmp`; the atomics of (1) also at an address
  (`rt.atomic_cas_at(a, off, ...)`); `rt.obj_addr[T](x) -> Int` and
  `rt.obj_at[T](a) -> T` (no count change; lowered by internal/ssa like
  the generic intrinsics at the end of rt.xo); the layout constants
  (string and list headers, element sizes) from one table the SSA builder
  also uses.
- The allocator and reference counts in Xo must not allocate or count
  inside themselves: compile them under the real-time check (decision
  0133 already proves a function allocation free) plus a module level
  rule that Int addresses are never counted.
- Lowering: plain loads and stores; `mem_copy` and friends inline for
  small constant sizes, else a call to the platform's memcpy (allowed:
  a platform library) or (4)'s Xo loop where there is none (Linux
  without libc).
- The allocator, `xo_rc_*`, and `xo_rl_*` move last within (2): every
  other C calls them.

### (6) Task switch and thread state (unblocks 1,166)

- `rt.stack_switch(save_addr, to_sp)` and `rt.fiber_start_addr()`: the
  emitters produce today's sequences (arm64 AAPCS64 x19 to x28, fp, lr,
  d8 to d15; Windows on Arm plus the TEB stack bounds; x86_64 SysV and
  Win64 with xmm6 to xmm15 and the TEB) as a function in the output
  (selfhost/arm64 assembly; module level asm in the LLVM emitters). wasm
  has no stack switching (its runtime runs tasks to completion today).
- Thread locals (the heap pointer, the current context): `rt.tls_get(k)`
  and `rt.tls_set(k, v)` over a fixed internal slot table. LLVM:
  `thread_local` globals. selfhost/arm64: Linux `mrs tpidr_el0` plus the
  local exec offset (selfhost's ELF writer needs a PT_TLS segment and
  the TLS relocation), macOS TLV descriptors (`__thread_vars`, a call of
  the descriptor's thunk; selfhost/macho needs the section). A cheaper
  later variant once no C remains on arm64: a reserved register for the
  current context, as Go's g; an internal ABI choice, not user visible.
- Fiber stacks: mmap with a guard page through (3); sanitizer fiber
  hooks (`__tsan_switch_to_fiber`, ASan fake stacks) as (3) calls in
  sanitizer builds.

### (4) SIMD and word scans (unblocks 313; rematches split, lines, rune_count, to_hex)

- `rt.bytes_find_byte(b, from, to, c) -> Int`, `rt.bytes_find(b, from,
  sub) -> Int`, `rt.bytes_eq_at(a, ai, b, bi, n) -> Bool`,
  `rt.bytes_count_cont(b, from, to) -> Int`, `rt.popcount(w)`,
  `rt.ctz(w)`, `rt.clz(w)`.
- Lowering: arm64 NEON 16 bytes a step (`cmeq`, `shrn` to a 64 bit
  mask, `rbit`/`clz`), `cnt` for popcount; LLVM `<16 x i8>` compares and
  `llvm.cttz`/`ctpop`; wasm SIMD128 where enabled, else SWAR over i64.
  The word at a time (SWAR) versions over `bytes_word` need only
  popcount/ctz and come first.

### (5) Constant time (unblocks 1,719 plus 509 held; the third party C)

- `rt.ct_select(m, a, b)`, `rt.ct_eq(a, b) -> Int` (all ones or zero),
  `rt.ct_lt`, and `rt.value_barrier(x)`: lowered to `csel`/masks behind
  an optimization barrier (an empty inline asm with a register operand
  in LLVM, an opaque MIR op in selfhost/arm64) so no pass turns them into
  branches; a native module function marked constant time (an internal
  marker, like the real-time check) where the SSA builder rejects
  branches and indexes on values derived from marked parameters, and
  bounds checks on secret indexes (tables scanned whole, as xo_p256.c
  does).
- A check on the output: a MIR taint pass in selfhost/arm64 that fails
  the build if a conditional branch or a load address depends on a
  marked value, plus a dudect style timing test over Ed25519 and P-256.
- AES-GCM and ChaCha20-Poly1305 for TLS need hardware AES and carry
  less multiply (`aese`/`aesmc`, `pmull`; AES-NI, PCLMULQDQ) as
  intrinsics; table AES is not constant time.
- The TLS stack itself: see owner decision A.

## Third party

All 200,915 lines are blocked by (5). Mbed TLS is TLS 1.3 and 1.2,
X.509, ASN.1, and PSA crypto in C; most of it (record layer, handshake
state machine, certificate parsing) is protocol code that would be (0)
or (3) in Xo, but it is a rewrite, not a port: the realistic route is a
TLS stack in Xo shaped like Go's crypto/tls and crypto/x509 (Go has had
no C there ever), with the primitives (P-256 from fiat-crypto's
generated Go or Xo output, X25519, Ed25519, AES-GCM, ChaCha20-Poly1305,
SHA-2) under (5). fiat-crypto generates Go and other languages from its
proofs, so an Xo target for fiat-crypto's generator (or a translation of
its Go output) keeps the formal verification.

## Owner decisions found

- **A. TLS end state.** (a) an Xo TLS stack after Go's design, replacing
  Mbed TLS (new, unaudited protocol code in Xo); (b) Mbed TLS stays as
  the one C exception behind its narrow boundary, which contradicts
  decision 0195's goal; (c) the platform's TLS where one exists
  (Security.framework, SChannel), which leaves Linux without one (it
  would need OpenSSL as a system library, a dependency Xo does not have
  today). This audit's order works for any of them; (5) is last.
- **B. Linux without libc.** Decision 0195 lists "raw system calls on
  Linux". Taken fully that means static binaries with no C library: an
  Xo DNS resolver in place of getaddrinfo (resolv.conf, /etc/hosts,
  nsswitch semantics differ), clone3 threads, and Xo's own TLS setup, as
  Go does. The other choice calls the system C library on Linux (glibc
  or musl, dynamic), which 0195's "the C library where the platform
  requires it" does not cover because Linux does not require it.
- **C. (lead or owner) Decision 0180 item 3.** The top down order keeps
  "C never calls compiled Xo". If a step wants a leaf (the allocator)
  in Xo while C still calls it, that needs C ABI exports of Xo runtime
  functions and an ADR superseding 0180 item 3. Internal, not user
  visible.
- **D. `use c` adapters.** Programs with `use c` bindings still need a
  C toolchain for their adapters (cffi/xo_<prefix>.c). Out of the runtime
  goal as written; confirm that "no C toolchain for the runtime" is the
  goal and not "no C toolchain for any Xo program".

No prerequisite needed a user visible feature: every sketch above is an
internal intrinsic or an internal marker on native modules.

## Next experiments, ranked

1. Delete the C paths that hold the bar once the lead's full tier
   passes: unicode (with its ADR), jsontext, tz, num (plus the shared
   82 lines), strings, report, show: 4,313 lines, one change per module.
   Done for unicode, jsontext, num, the shared 82, and strings (decision
   0198, 2026-10-13: 3,354 lines); tz, report, and show (959) wait for a
   full tier of step 5.
2. (4)'s SWAR half now: `rt.popcount` in the three self hosted emitters
   and rune_count over `bytes_word`; remeasure with
   `bench/perf/text/measure.sh` (`WORKLOADS=rune_count`).
3. The (0) groups under decision 0193's rules: sized integer operations
   (xo_bytes.c 1-200, 0193 step 5 next (b)), glob matching (xo_testing.c
   1305-1498), the crypto hashes (xo_crypto.c 129-671, 964-1077) with a
   bench/perf/crypto program first, since their bar is unmeasured.
4. The ADR for (3), starting with `rt.sys6` and `native/sys_darwin.xo`
   on selfhost/arm64, moving xo_rt.c's environment (7014-7070) and
   xo_bytes.c's fs_read (703-818) as the pilot pair.
5. The ADR for (1) on `List[Int]` slots, piloted on Atomic[T]
   (xo_rt.c 4881-4937) and the test world's virtual clock.
