# 0195: No compiled C in the runtime

Status: Accepted as a direction (owner, 2026-10-12); the plan and its
steps are Proposed until the audit below lands.

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
which prerequisite holds each piece, in bench/results/.

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
