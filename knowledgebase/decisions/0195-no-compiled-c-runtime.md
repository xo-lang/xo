# 0195: No compiled C in the runtime

Status: Accepted as a direction (owner, 2026-10-12). The audit landed
(2026-10-13, `bench/results/zero-c-audit-2026-10-13.md`), refreshed at
d79ad79b as a finite checklist with an order of agent sized items
(`bench/results/zero-c-audit-refresh-2026-10-13.md`: std/llrt 26,743
lines, (a) 2,060, (b) 24,678 in 31 items, (c) 5, third party
200,915); the per step
plan below ("Plan") is Proposed until the owner approves it and answers
owner decisions A to D. The "(0) portable now" group moved (2026-10-13,
item 6: 1,040 of its 1,799 lines out of default builds,
`bench/results/runtime-port-portable-2026-10-13.md`). Work item B08
done 2026-10-14 (SHA-512 and Rand in Xo; the C Rand stays until the
seed has B08; `bench/results/sha512-rand-2026-10-14.md`). Work item
B10 done 2026-10-14 (the HTTP/1.1 client's response reading,
redirects, and URLs in Xo, the HTTP/2 server's :path parse from Xo;
1,629 runtime C lines deleted; client 0.953 to 1.001 of the C's
instructions; `bench/results/http-client-2026-10-14.md`). Work item
B11 done 2026-10-14 (the HTTP/2 client's HPACK in Xo, xo_h2.c's C HPACK
deleted, 786 runtime C lines deleted and 242 added; HTTP/2 client 0.982
to 1.009 of the C's instructions;
`bench/results/http2-client-hpack-2026-10-14.md`). Work item B13 stage 1 done 2026-10-14 (http1.xo's searches
through scan.xo, xo_str_find deleted; the string and list objects
measured in Xo and kept C: arm64 1.27 times the C on bench/perf/alloc
small without shrink wrapping; `bench/results/string-list-objects-2026-10-14.md`).
Work item B12 done 2026-10-14 (the HTTP/1.1 response writer in
Xo, std/native/llrt/http1w.xo, which HTTP/2's h2_head uses too;
`bench/results/http-writer-2026-10-14.md`). Work item B14 done 2026-10-14 (the test world's part 1 in
Xo, std/native/llrt/testworld.xo; 916 runtime C lines deleted, 87
added; `bench/results/test-world-2026-10-14.md`). Work item B16 part 1
done 2026-10-14 (the Fs operations past read, write, and stat in Xo
on Linux and darwin/arm64, std/native/llrt/fsops.xo, with the flock
wait through `rt.clock_sleep_or_cancel`; 0.683 to 1.003 of the C's
instructions; no C deleted until part 2, Windows;
`bench/results/fs-ops-2026-10-14.md`). Prerequisite 5
(constant time) done 2026-10-14: decision 0222 (`rt.ct_barrier`,
std/native/llrt/ct.xo; crypto.equal and Ed25519 in Xo, work item B09,
below the C's instructions on both backends). Prerequisite 4 (SIMD and word scans) done
`bench/results/http-writer-2026-10-14.md`). Work item B03 done 2026-10-14
(standard output's buffer and standard error in Xo,
std/native/llrt/stdio.xo, 0.788 to 1.005 of the C's instructions;
exit, env, and args in std/native/llrt/process.xo; the C they replace
goes with the next seed; `bench/results/stdio-2026-10-14.md`). Prerequisite 4 (SIMD and word scans) done
2026-10-13: decision 0201. Prerequisite 3 (system calls) on Linux:
stage 1 done (decision 0206, 2026-10-13: `rt.sys6`,
std/native/llrt/linux.xo, files routed, static programs with no libc
under XO_NOLIBC=1); stage 2 partly done 2026-10-13 (the vDSO clock,
futex locks, signals, threads, and the TLS layout in Xo, xo_nolibc.c
1,859 to 1,622 lines); unwinding and stage 3 (sockets, DNS) open.
Prerequisite 3 on macOS and Windows:
stage 1 done (decision 0213, 2026-10-13: platform library calls from
Xo through std/native/sys_darwin.xo and sys_windows.xo, the libSystem
and kernel32 layers darwin.xo and windows.xo, files routed on
darwin/arm64 and Windows with the C path's messages byte for byte;
Clock.now stays C until the arm64 emitter folds a `match` on `target`).
Prerequisite 2 (raw memory) stage 1 done
2026-10-13: decision 0207 (raw memory intrinsics, C ABI exports; the
"(0) held by C callers" group moved but the test world stores, which
are (1) and (2); `bench/results/raw-memory-2026-10-13.md`); stage 2's
reference counts moved to Xo (std/native/llrt/rc.xo, XO_LLRT_C=rc for
the C) and lists of zeros take one allocation (raw.zero_ints); stage 3
moved the allocator (std/native/llrt/alloc.xo, XO_LLRT_C=alloc for the
C; the heap pointer stays a C thread local, (6), with the intrinsics
designed in decision 0207). Argon2id and BLAKE2b stayed C (1.155 of the
C on LLVM, 0.980 on arm64) until decision 0211's alias scopes brought
LLVM to 1.043 (2026-10-13): they are Xo by default, the C under
`XO_LLRT_C=digest`. Prerequisite (6) stage 1 done 2026-10-13: decision
0212 (the fiber switch and thread locals emitted by the compiler, the
scheduler core in std/native/llrt/sched.xo, `XO_LLRT_C=sched` for the C;
xo_rt.c's default build 156 lines shorter, no assembly;
`bench/results/task-switch-2026-10-13.md`); stage 2 done 2026-10-13:
the worker pool, the locks' slow paths and futex waits, worker threads,
fiber stacks, parks, the cancellation tree, a spawn's queueing, and a
scope's join (std/native/llrt/pool.xo; the default build of xo_rt.c 523
lines shorter than main's at 4f792c55, 7,088 to 6,565); await, the wait
lists, the task's record and end, contexts, and budgets are 0212's
stage 3.

## Context

After decision 0193's port, the native runtime still compiles C: the
scheduler, atomics, system calls, TLS (third_party/mbedtls), the bulk
byte scans (split, lines, rune_count), and, until decision 0194 lands,
Duration text. Every C file needs a C toolchain per target or a
prebuilt runtime pack per target (decision 0157), a second language in
runtime faults, and code outside Xo's checks. Go has had no C in its
runtime since Go 1.5, but still calls platform libraries where the
kernel interface is not stable (macOS libSystem, Windows DLLs, OpenBSD).

## Decision

The goal is no C that Xo compiles for the runtime. Calling the
platform's own libraries (libSystem, kernel32, the C library where the
platform requires it) is allowed. It is queued after decision 0193's
runtime port. Each piece moves under 0193's rules: its C path stays
selectable (`XO_LLRT_C`) until the Xo version meets decision 0180's bar
on both native backends.

Prerequisites, each its own decision when started:

1. Atomics and memory ordering intrinsics in the SSA and both backends
   (unblocks the scheduler). **Done** (decision 0200, 2026-10-13):
   `rt.slot_*`, `rt.fence`, `rt.spin_hint` on List[Int] and Atomic
   cells in every emitter, litmus tested on arm64 and x86-64; Atomic[T]
   ported within the bar (its C path behind `XO_LLRT_C=atomic`).
2. Raw system calls on Linux, and direct libSystem calls on macOS, from
   Xo.
3. SIMD or word at a time intrinsics so bulk scans can match memchr.
4. A way to keep constant time code constant time (no secret dependent
   branches or loads through the optimizer), before TLS and crypto
   leave C; until then mbedtls stays behind a narrow boundary, last.

First step: an audit counting the C in std/llrt and third_party by
which prerequisite holds each piece, in bench/results/ (done:
`bench/results/zero-c-audit-2026-10-13.md`).

## Plan (Proposed, from the audit of 2026-10-13)

The audit splits prerequisite 2 into (3) system and platform calls and
(6) the per CPU task switch with thread local state, and numbers them:
(1) atomics, (2) raw memory, (3) system and platform calls, (4) SIMD and
word scans, (5) constant time, (6) task switch and thread state; (0) is
portable now under decision 0193's bar.

1. **Counts.** Native builds compile 231,584 lines of C: std/llrt 30,669
   (15 files, one translation unit from xo_rt.c), third_party/mbedtls
   196,689 (.c 120,810 raw, much compiled out by the configuration; .h
   74,707; xo/ 1,172), fiat-crypto 4,226. std/llrt by what unblocks each
   line (the last prerequisite it needs in the order of item 2): in
   progress (decision 0193 step 6) 4,567; C paths kept behind
   `XO_LLRT_C` 4,654; (0) 1,799 and (0) held by C callers 1,806; (3)
   3,105; (1) 4,966; (2) 6,574; (6) 1,166; (4) 313; (5) 1,719. All
   third party C is (5).
2. **Order**, top down so decision 0180 item 3 (C never calls compiled
   Xo) holds at each step: delete the C paths that hold the bar (4,313
   lines: unicode, jsontext, tz, num, strings, report, show) after a
   full tier; finish 0193 step 6 (4,567); move (0) (1,799; the 1,806 held
   lines move with their callers); then (3) 3,105, (1) 4,966, (2) 6,574,
   (6) 1,166, (4) 313 (independent, any time), (5) last with 1,719 plus
   the TLS stack. Text's C path (341) stays until Duration text passes
   (decision 0194). Done for unicode, jsontext, num, strings, and the
   82 lines only num and jsontext called (decision 0198, 2026-10-13:
   3,354 of the 4,313 lines; 3,373 lines of std/llrt with the blank and
   comment lines around them); tz, report, and show (959) went in its
   second round (2026-10-13, after the batch 8 full tier), with the C
   paths added since the audit (digest, ints but its intrinsics, glob,
   alloc, fspath, gen, rc, atomic, procspec, http1, hpack, and the
   scan and sysio switches): std/llrt 29,916 to 26,693 lines (3,299
   deleted, 76 added). Left selectable: text (514 lines, Duration
   text), sched (481) and net (no full tier yet).
3. **Intrinsics**, all internal (declared for `use native/rt` modules,
   decision 0074; no user visible change), lowered in selfhost/arm64,
   selfhost/llvm, selfhost/wasm first and mirrored in internal/arm64 and
   internal/llvm: (3) `rt.sys6` (Linux), per OS declaration files whose
   functions lower to bare C ABI symbols (libSystem; kernel32, ntdll,
   ws2_32), `rt.bytes_addr`, `rt.bytes_put_word`, C ABI entry points for
   platform callbacks; (1) atomic load, store, swap, cas, fetch add,
   fence, spin hint on a `List[Int]` slot with constant orderings; (2)
   loads and stores at an Int address, mem copy/move/set/cmp, object
   address and back, one table of layout constants, the allocator and
   counts compiled under the real-time (allocation free) check; (6)
   `rt.stack_switch` emitted per CPU and OS, thread local slots (LLVM
   `thread_local`; ELF TLS and Mach-O TLV in selfhost's writers); (4)
   find byte, find, compare, continuation byte count, popcount, ctz,
   clz (NEON, LLVM vectors, wasm SIMD128 or SWAR); (5) constant time
   select, compare, value barrier, a constant time marker the SSA builder
   enforces, a MIR taint check, AES and carry less multiply intrinsics.
   Each becomes its own ADR when started, as decided above.
4. **Lost the bar, rematch**: split and lines by (4) plus (2)'s inline
   copy; rune_count by a popcount over `rt.bytes_word` (possible before
   the rest of (4)); to_hex by (4) or a 32 byte push; Duration text by
   decision 0194's list, not by these prerequisites. **Done for split,
   lines, rune_count, and to_hex** (decision 0201, 2026-10-13): without
   (2)'s inline copy (a piece is still one rt.str_slice call), split
   0.858x and 0.977x, lines 0.984x and 0.989x, rune_count 0.326x and
   0.323x, hex 1.011x and 0.942x of the C on arm64 and LLVM
   (bench/results/scan-intrinsics-2026-10-13.md).
5. **Owner decisions** (open): A, the TLS end state (an Xo TLS stack
   after Go's crypto/tls, or Mbed TLS kept as the one exception, or the
   platform TLS where one exists, which Linux lacks); B, Linux without
   libc (an Xo DNS resolver for getaddrinfo, clone3 threads) or the
   system C library on Linux; C, whether a step may export Xo runtime
   functions to C (superseding 0180 item 3) instead of the top down
   order; D, whether `use c` programs needing a C toolchain for their
   adapters is outside the goal.

6. **The "(0) portable now" group** (2026-10-13, decision 0193's
   mechanism and bar; results `bench/results/runtime-port-portable-2026-10-13.md`):
   - Moved, 1,040 of the group's 1,799 lines out of default builds (996
     under `XO_INTS_C`, `XO_GLOB_C`, `XO_DIGEST_C`, `XO_GEN_C`, 44 of
     dead `xo_str_quote` deleted): sized integer operations and
     conversions lowered inline by both SSA builders, their faults,
     UInt64 division, floor division, the Str key hash, and Int's
     checked_ and saturating_ methods in std/native/llrt/ints.xo
     (`XO_LLRT_C=ints`); the C versions of the intrinsics compiled only
     when called; the UInt64 and Float32 conversions as inline
     intrinsics; MemFs.fail_next's path.Match in glob.xo (`glob`);
     SHA-256 (generated unrolled), HMAC, PBKDF2, unpadded base64, and
     verify_password's parsing in digest.xo (`digest`); the scalar
     values of property tests and testing.gen in gen.xo (`gen`). New
     internal intrinsics: `rt.int_wrap` ops 5 to 7 (rotate, rotate the
     low 32 bits, byte reverse).
   - The bar holds on every workload (instructions less setup, Xo over
     C, arm64 and LLVM): sized arithmetic 0.050 to 0.209, `str_hash`
     1.038 and 1.032, `unix` 0.578 and 0.797, `checked` 0.752 and 0.507,
     Float32 0.756 and 0.556 (against the compiler before the
     intrinsics), SHA-256 0.820 and 0.675, HMAC 0.987 and 0.826, PBKDF2
     0.932 and 0.755, glob 1.009 and 0.884, generated values 0.946 and
     0.888.
   - Stays C, 759 lines (BLAKE2b and Argon2id moved to Xo on
     2026-10-13, decisions 0208 and 0211, at 0.980 of the C on arm64 and
     1.043 on LLVM): BLAKE2b and Argon2id (an Xo version retired
     2.071x the C's instructions on arm64 and 1.294x on LLVM and was
     taken out; the arm64 code shape for List[Int] heavy code is the
     next step), SHA-512 (held by Ed25519, group (5); decision 0204 now
     lets the C Ed25519 call an Xo SHA-512), the generator's
     state, record, and replay (calloc, realloc, getenv: groups (2) and
     (3)) with the test Rand that crypto's C calls, and file structure
     (includes and headers, 51).
   - Found on the way: internal/arm64 read a register before writing it
     for a function inlined twice under a `&&` loop condition
     (gotchas/arm64-and-loop-condition-inlined-twice.md, repro inside);
     a UInt64 shift amount of 2^63 or more shifts on the Go backend and
     faults natively (gotchas/uint64-shift-amount-above-2-63.md).

## Consequences

- New targets need only Xo's emitters and linkers, not a C toolchain.
- Runtime faults are Xo source end to end; the scheduler and I/O code
  get Xo's checks.
- Order: runtime port (0193), intrinsics (1 to 3), scheduler, I/O,
  crypto (4) last.

## Alternatives

- Keep C for the runtime permanently: rejected as the goal; the
  prebuilt packs make it workable for users today, but it keeps clang
  as a build dependency and a second language in the runtime.
- Zero C including platform libraries (raw system calls everywhere):
  rejected; macOS and Windows do not support it.
