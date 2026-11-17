# 0696: the native Rand maps a span below 2^32 by Lemire's multiply

Status: Accepted (owner, 2026-11-17; proposed by Xo Lang 1, 2026-11-17, task #87). Supersedes decision 0669's alternative "Lemire's multiply for
`rand.int`: changes the seeded streams the Go backend and tests rely on" for the draws it covers; the rest of 0669
stands. If the owner rejects this decision, the push that carries it is reverted.

## Context

After decision 0669 (the per-thread ChaCha8 buffer), bench/perf/random perm (2,000 permutations of 1,000, one
`r.int(0, i + 1)` a swap) was still 1.26 to 1.35 of Go's math/rand/v2 Perm on the Mac (twincheck -time 7). Perm is
the draw's remainder and the list: llrt `next_int` maps a word to a span with an unsigned remainder, a division per
draw, where Go's `uint32n` and `uint64n` use Lemire's multiply (the high half of word times span, the low half
checked against (2^k - span) mod span only when it is below span).

0669 refused Lemire because the mapping fixes which values a seeded stream gives: the test world's seeded generator
(a test binary's `os.rand`) and the Go backend's std/rt randRange keep the remainder, and changing it changes every
recorded seeded stream. But the words 0669 added, the worker's ChaCha8 buffer, are never the test world's (next_u64
fills a buffer only after the test world check, so in a test binary every buffer stays empty) and are seeded from
the OS: no one can replay them, so how they map to a span is not observable.

## Decision

1. `next_int` (std/native/llrt/rand.xo) with 0 < span <= 2^32 - 1 takes its word from the worker's buffer when the
   buffer has one and maps it as Go's 32 bit Lemire: x = (word mod 2^32) * span, the result lo + x / 2^32, a draw
   rejected when x mod 2^32 < (2^32 - span) mod span (the remainder computed only when x mod 2^32 < span, about
   span / 2^32 of draws).
2. Everything else keeps the remainder mapping: a span of 2^32 or more, the full word range (normal's and exp's
   draw, no division already), a thread that is not a worker, the slow path (a refill, the seeding draw, the OS
   generator), and the test world's seeded draws. Seeded streams, `random.seeded`, the Go backend and wasm are
   unchanged.
3. Uniformity is the same (each value of the span from exactly floor(2^32 / span) accepted low words); the
   rejection rate is below span / 2^32, at most one in 2^32 / span draws, against the remainder's 2^64 based one.

## Consequences

- bench/perf/random perm on the Mac (twincheck -time 7, 2026-11-17): 25.8 to 15.8 ms, 1.352 to 0.813 of Go; normal,
  exp and zipf unchanged (within noise). Host 1 (linux/arm64, `xo bench --ab`, 3 rounds, 2026-11-17): perm 23.61
  to 10.19 ms (B/A 0.432, 95% interval 0.417 to 0.438).
- The low 32 bits of a buffered word are used, the high 32 dropped; a ChaCha8 word's halves are equally random.
- Test: internal/llvm/testdata/golden/rand_int_lemire (arm64, LLVM): spans 1, 3 (in a task), 7, 1000, 2^32 - 1 and
  2^32 in range and near uniform (bounds 6 standard deviations wide), every value of a 1,000 span drawn.

## Alternatives

- 64 bit Lemire (Go's uint64n) for every span: needs an unsigned 64 by 64 high multiply in llrt (only
  `mul_hi_signed` exists) and would cover spans no benchmark draws; a later step if one does.
- Lemire for seeded streams too: changes recorded test and Go backend streams (0669's reason, still right there).
