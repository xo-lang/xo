# 0060: Native arm64 latch copies at inner loop guards; loop bounds checks stay

Status: Accepted (2026-10-08). Extends decision 0054. The "loop bounds checks stay" part superseded by 0184 (2026-10-12). Implemented in
`internal/arm64/lsr.go` (`copyable`, `dupLatch`, `emitLatch`, marked by
`placeLatches`) with a hook in the conditional branch emission of
`internal/arm64/emit.go`. Evidence: `bench/results/arm64-perf-2026-10-08.md`
(section "sieve and nbody, 2026-10-08, third pass").

## Context
After decision 0054 sieve ran at 1.02 to 1.03x of the LLVM backend in
cycles with the same instruction count as before. An instruction level
profile (xctrace) split the gap: the inner loop and the push loop within
noise of LLVM's, and the phase past sqrt(n), where the outer loop only
tests flags and every prime found skips the inner loop (`j = i * i > n`),
33 percent instruction delivery bound against LLVM's 13 percent. Each such
prime took three taken branches after the mispredicted flag test: into
the prime path, the inner loop guard to the latch, the latch back to the
test. Separately, nbody's slowdown from bounds check removal (0054) was
left without a cause.

## Decision
- **Latch copies**: a latch block that `placeLatches` moved (and the move
  only blocks on the way to it) is marked copyable when it has at most
  four instructions, moves or plain instructions without fault exits,
  then a jump. A conditional branch to a marked block whose other target
  is the next block is emitted as a branch to that next block followed
  by a copy of the latch (its instructions and its rotated back edge).
  In sieve the zero trip guard of the inner loop now falls into the
  copy: one taken branch (the back edge) on that path instead of two.
  The path into the inner loop takes the branch instead.
- **Loop bounds checks stay** (0054 unchanged): the nbody slowdown is a
  layout effect of the loop's size; no size of the loop without the
  checks is faster than the original
  (`gotchas/arm64-fewer-instructions-slower.md`).

## Consequences
- sieve: 1.4 percent fewer cycles (paired median of 101 interleaved
  rounds), now within 0.3 to 1 percent of LLVM. The other six bench
  programs' assembly is byte for byte the same.
- A copy grows the function by at most four instructions per marked
  branch; the latch is emitted twice, once in place for the other paths.
- New run program `internal/arm64/testdata/run/latch` (inner loops that
  run zero times, once, many times; more loop carried values).

## Alternatives
- **LLVM's layout** (latch before the header, the flag test branching
  back to it, the prime path falling through): no gain over the copy in
  cycles (sieve 409.6 against 404.1M, s0 406.7M, 101 rounds), though its
  phase past sqrt(n) matched LLVM's per instruction delivery.
- **The latch copied also at the inner loop's exit**: not better than the
  guard alone within noise; left out.
- **The push loop counting down to zero** (`subs`/`b.ne`, LLVM's shape):
  50M fewer instructions, no cycle change measurable through the noise
  (gotchas/bench-sieve-cache-noise.md).
- **Unchecked `count += 1`** in sieve's outer loop (its bound proves no
  overflow): no measurable change; not built.
- **Re-enabling the bounds check removal with padding** to the loop's
  original size: ties, never wins on nbody, and padding to a size is a
  layout bet that would not carry to other loops.
