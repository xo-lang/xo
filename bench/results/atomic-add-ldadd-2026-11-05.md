# Atomic[Int].add: one ldadd instead of a compare and swap loop (2026-11-05)

micro13's spawn100k (100,000 tasks of `total.add(i % 7)` in one scope) was the
one micro13 row where Xo trailed Go on every platform in the 0.39 precut report
(bench/results/release-vs-go-v0.39.0-precut.md). On main after decision 0507
(ba2eeadbb) the wall gap was gone on arm64 but Xo used 1.3 to 2.1 times Go's CPU.

Where it went (linux/arm64 host 1, an Apple CPU under Linux; `perf record -g
--user-regs=lr`, since xo's profiler gives the outline atomic helpers no
caller): 84.6% of all CPU samples were `__aarch64_cas8_acq_rel` called from the
task body. llrt atomic.xo's `add` was a load and compare and swap loop (it must
fault on overflow and store nothing), and with twelve workers adding to one
counter every compare and swap but one failed and retried. Experiments
(spawnprof, 10 x spawn100k): tasks that capture the counter but do not add
used 0.80 s of user time, like tasks that capture nothing (0.82 s), against
3.6 s with the add; so neither the closure's count nor the scope's pending
count was the cost.

Change: `add` does one fetch and add (`rt.slot_add`, ldadd with LSE) when
the operand is under 2^20 and the value it loads is within 2^62 of zero; the
checked loop otherwise. Why it cannot overflow is in the code's comment.
The atomic_ops golden gained the bounds (an operand of exactly 2^20, values
on both sides of 2^62, a negative overflow caught by task.isolate that leaves
the value unchanged).

`xo bench --ab`, 15 rounds:

| platform | A | B | wall | cpu | work_ms (micro13's own) |
|---|---|---|---|---|---|
| linux/arm64 (ab-host, load 1.5) | main ba2eeadbb | change | 0.911 (0.884 to 0.977) | 0.754 | |
| darwin/arm64 (Mac, measure class, load 6.9) | main | change | 0.783 (0.758 to 0.808) | 0.477 | 0.724 (29 to 21 ms) |
| darwin/amd64 (Intel laptop, load 1.1) | main | change | 0.996, noise | 0.999 | 1.0 (29 ms) |
| linux/arm64 (host 1, load 5.6) | Go 1.26.1 | change | 0.766 | 0.994 | 0.767 (30 to 23 ms) |
| darwin/arm64 (Mac, load 6.9) | Go 1.26.1 | change | 0.856 | 1.176 | 0.808 (26 to 21 ms) |
| darwin/amd64 (laptop) | Go 1.26.1 | change | 1.018 | 1.35 | 1.077 (26 to 28 ms) |

Main against Go before the change (work_ms): linux/arm64 22 vs 24 (0.917,
noise; cpu 1.488), darwin/arm64 28 vs 28 (cpu 2.105), linux/amd64 13 vs 13
(cpu 1.415), darwin/amd64 28 vs 26 (1.077; cpu 1.34). x86's lock cmpxchg loop
is not much worse than lock xadd, so amd64 is unchanged; the laptop's
remaining gap is idle workers sleeping and being woken (59% of its profile in
_ulock_wait, 36,000 futex waits per 10 runs against 14,000 on linux/arm64).
