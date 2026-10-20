# 0273: --race on the native backends with a prebuilt TSan runtime

Status: Proposed (owner decision 2026-10-19, through the lead; Xo Lang 2).
Stub claiming the number; filled in as the work lands.

## Context

`--race` builds with LLVM today (decision 0270: "--race picks LLVM"),
which needs clang and its ThreadSanitizer runtime on the machine. The
owner wants `--race` on Xo's own native backends, shipped the way Go
ships its race detector: a prebuilt runtime in the release archive and
seeds, linked by Xo's own linkers, so `--race` needs no clang.

## Decision (to be written)

- The runtime: LLVM compiler-rt's ThreadSanitizer (C++), at a pinned
  revision, built per platform by a maintainer script (never on users'
  machines), shipped in lib/xo/race/<goos>-<goarch>.
- License: Apache 2.0 with the LLVM exception, in THIRD_PARTY_NOTICES;
  the one non-Xo component of a build, linked only with --race.
- Instrumentation by the native emitters: loads and stores of escaping
  memory (__tsan_read/write), function entry and exit, atomics and
  reference count operations as TSan atomics; the runtime's tsan()
  hooks; the C library path on Linux for --race.

## Consequences, Alternatives

To be written with the results of the steps (hand-instrumented
experiment, arm64 emitter, amd64, packing).
