# 0727: after std Proven, performance work is compiler wide, not per module

Status: Accepted (owner, 2026-11-20)

## Context

Decision 0461 makes a std module Proven on performance when every bench
twin row runs at or under 1.05 of Go. Reaching that bar took per module
tuning (brotli's hash and zopfli loops, std/go's literal bytes) beside
compiler work (arm64 bounds proofs, #457; loop splitting, 0724). The
owner asked (2026-11-20) whether Xo could run most things in 0.5 to 0.75
of Go's time, and where optimization stops paying.

Figures already under 1.0 on linux/arm64 host 2: brotli enc1 0.709,
enc6 0.930, decoders 0.88 to 1.02 (std/evidence/brotli/perf.md,
b7e9b20e0 and 7b2798503); std/go bytes 0.839 by host 1 `--ab` (#467,
04eaa39a1). CPU bound kernels (compression, hashing, parsing, encoding,
math) have room, since Go's compiler trades optimization for compile
speed (limited inlining, few bounds check removals, little automatic
vectorization). Allocation, GC, syscall, network and lock bound code has
little: Go is mature there and the floor is the OS and hardware. Xo also
checks integer adds for overflow, a cost Go does not pay (#423).

## Decision

1. Per module tuning stops at the Proven bar (1.05 of Go, 0461, read as
   0726 says). A std module is not tuned further for its own rows.
2. Performance work after Proven is compiler wide: bounds and overflow
   check elimination, inlining, register allocation, loop and vector
   work, codegen gaps found by the #392 to #395 checks. Lang 1 owns it
   (xw decision #202).
3. A compiler wide change is scored on a broad benchmark set (every std
   bench twin plus the codegen corpus), not on the one module that
   motivated it, and reports its compile time cost (`xo build`
   phase times on compiler/xo). A change that costs noticeable compile
   time goes behind an option or the LLVM backend only.
4. Every check removing optimization carries a proof and a test that a
   needed check stays (an add that can overflow keeps its check).

## Consequences

- Effort moves from many single module wins to fewer changes that speed
  up every program.
- A broad scorecard and a compile time budget are needed before the
  first such change lands (task for xw-1: one command that runs the
  set and prints per row ratios plus compile time).
- Speed claims still need `xo bench --ab` (0340).

## Alternatives

- Keep tuning each module toward 0.5 to 0.75 of Go: high cost per row,
  no gain for user programs, more code to keep correct.
- Stop all performance work at Proven: leaves the CPU bound headroom
  above unused.
