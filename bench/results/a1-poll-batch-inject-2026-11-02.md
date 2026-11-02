# a1: a poll's batch past its first fiber injected (decision 0324, 2026-11-02)

Xo Lang 4. The owner chose P (poll batches only) as the Linux default
on 2026-11-02; R (reader grants) is parked, and loses on bench/perf/rwread (below). Base for the experiment rows:
4a16f4d80; P 4361a2ba2, R e21aedbce. Landed branch: l4-p.

## bench/perf/http (CRUD server, warm-up, 5 rounds, medians)

| host | change | c32 | c256 | c1024 |
|---|---|---|---|---|
| linux/arm64 (first host, exclusive) | P | req/s -0.8%, CPU a request -5.9% (ranges apart), p999 -3.6% | level, p999 -9.5% | req/s +2.7% (apart) |
| linux/arm64 | R | req/s +4.1%, CPU -4.3% (apart), p999 +5.2% | level | level |
| linux/amd64 (offload host, exclusive) | P | req/s -3.4%, CPU +0.6% | | +5.4% |
| linux/amd64 | R | req/s +10.9%, CPU -9.9% (apart) | +8.2% | +9.0%, p999 -9.1% |
| windows/amd64 (pinned) | P | +9% | | worse, load generator errors on both sides |
| windows/amd64 (pinned) | R | level | level | -17%, errors on both sides |

## xo bench --ab (linux/arm64)

| change | program | result |
|---|---|---|
| P | bench/perf/httpstreamsrv | CPU 0.886, RSS 0.94 |
| R | bench/perf/httpstreamsrv | wall level, CPU 1.006 (amd64: RSS 1.117) |
| P, R | bench/perf/conc/xo/rwmutex, 15 rounds | level (1.006, 1.009): the bench runs about 6 ms with CPU equal to wall, no parallel readers |
| P, landed branch against its parent | bench/perf/httpstreamsrv | CPU 0.893 (0.883 to 0.913), wall 0.992, RSS 0.96 within noise (a144a3115 against 7e5f1c0d1, 3 rounds, host load 8 on both sides) |

## Gate (default change)

linux/arm64 second host, d424e0c5b: internal/arm64 TestGoldenRun (134 s),
internal/stdtest (1312 s), bootstrap fixed point (s1 72 s, s2 126 s,
s3 119 s): pass.

Raw outputs: ~/xo-lead/notes/xo-lang-4/out/a1p*.txt, a1r*.txt, pgate.txt
(the agent's notes, not in the repository).

## Reverted (2026-11-02)

bench/perf/jitter /alloc, first linux/arm64 host, one job: req/s 132,426 against 100,127 at 32 connections (0.756, ranges apart) and 134,645 against 120,487 at 256 (0.895), CPU a request 1.037 and 1.064; p99 at 256 2.9 ms against 6.2 ms (one job, l4pts12: a144a3115 against 25d23d18e, 3 rounds, quiet host).

## R on bench/perf/rwread (2026-11-02)

The benchmark with reader contention that R waited for (decision 0324):
bench/perf/rwread (23a650da6), 256 tasks, 20,000 accesses each on one
`Mutex` (`with_read` sums 16 entries of a 4,096 entry table; writes hold
the lock for 200 work steps), A main 23a650da6, B R on it (263abb734, a
local commit: grant_readers' fibers past the first injected, polls
unchanged). Cross-built on the Mac, measured on the first linux/arm64
offload host (a Linux VM on an M4 Pro, 12 CPUs, exclusive lease, quiet
wait to load 0.95), 5 rounds alternating, medians with ranges in the job's
output; RSS from GNU time.

| threads | writes / 1,000 | reads/ms A | reads/ms B | B/A reads/ms | B/A wall | B/A CPU | B/A RSS |
|---|---|---|---|---|---|---|---|
| 1 | 1 | 8,106 [8,067 to 8,119] | 8,119 [8,093 to 8,183] | 1.002 | 0.998 | 0.999 | 0.999 |
| 1 | 10 | 7,410 [7,378 to 7,432] | 7,421 [7,399 to 7,464] | 1.001 | 0.999 | 0.999 | 1.000 |
| 1 | 100 | 3,813 [3,754 to 3,960] | 3,803 [3,763 to 3,848] | 0.997 | 1.002 | 1.002 | 0.999 |
| 2 | 1 | 9,183 [8,880 to 9,283] | 9,454 [9,053 to 9,525] | 1.030 | 0.971 | 0.967 | 1.020 |
| 2 | 10 | 2,586 [2,495 to 2,637] | 2,656 [2,541 to 2,722] | 1.027 | 0.970 | 0.949 | 1.041 |
| 2 | 100 | 1,424 [1,407 to 1,442] | 1,059 [1,040 to 1,064] | 0.744 | 1.344 | 1.279 | 0.999 |
| 4 | 1 | 5,825 [5,779 to 5,892] | 5,727 [5,676 to 5,773] | 0.983 | 1.017 | 1.034 | 0.981 |
| 4 | 10 | 2,611 [2,493 to 2,666] | 2,683 [2,498 to 2,710] | 1.028 | 0.973 | 1.149 | 1.000 |
| 4 | 100 | 1,426 [1,419 to 1,433] | 1,065 [1,045 to 1,084] | 0.747 | 1.338 | 1.153 | 1.019 |
| 8 | 1 | 4,595 [4,522 to 4,637] | 4,654 [4,616 to 4,666] | 1.013 | 0.988 | 0.963 | 1.035 |
| 8 | 10 | 2,667 [2,567 to 2,732] | 2,875 [2,657 to 2,909] | 1.078 | 0.929 | 1.116 | 1.017 |
| 8 | 100 | 1,428 [1,418 to 1,452] | 1,047 [1,040 to 1,053] | 0.733 | 1.363 | 1.169 | 1.036 |
| 12 | 1 | 4,364 [4,338 to 4,455] | 4,383 [4,327 to 4,443] | 1.004 | 0.996 | 0.981 | 1.017 |
| 12 | 10 | 2,703 [2,600 to 2,728] | 2,845 [2,698 to 2,883] | 1.053 | 0.950 | 1.144 | 1.000 |
| 12 | 100 | 1,437 [1,424 to 1,441] | 1,046 [1,035 to 1,051] | 0.728 | 1.373 | 1.173 | 1.017 |

R loses where the lock is contended most: at 100 writes in 1,000, reads a
millisecond fall to 0.73 to 0.75 of main at every thread count from 2 up
(ranges apart: A 1,407 to 1,452, B 1,035 to 1,084), wall 1.34 to 1.37,
CPU 1.15 to 1.28. At 10 writes it reads 1.03 to 1.08 faster in wall but
spends 1.12 to 1.15 more CPU from 4 threads up; at 1 write and at one
thread it is level. RSS is level everywhere. So R does not go to the
owner. A likely cause, not measured: injected reader grants spread the
readers across workers, which then contend on the lock with the next
writer, where on the granting worker they run back to back. R stays
parked (the agent's ref refs/recover/l4-a1r2).
