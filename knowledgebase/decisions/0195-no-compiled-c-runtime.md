# 0195: No compiled C in the runtime

Status: Accepted as a direction (owner, 2026-10-12). The audit landed
(2026-10-13, `bench/results/zero-c-audit-2026-10-13.md`); the per step
plan below ("Plan") is Proposed until the owner approves it and answers
owner decisions A to D. Prerequisite 4 (SIMD and word scans) done
2026-10-13: decision 0201.

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
   (unblocks the scheduler).
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
   (decision 0194).
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
