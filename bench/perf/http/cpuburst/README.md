# cpuburst: connection bursts while handlers are CPU bound

A small server (Xo in xo/, Go in go/) with GET /users/1 and GET /cpu
(about 86 ms of compute, no runtime calls), and run.sh, which measures
loadgen's burst mode (200 connections at once, 10 rounds) while N curl
loops keep /cpu busy. Xo runs twice: A serves in main's body (as every
server does), B in a spawned task (main parked at the scope's end). Go
is the reference.

```
XO=bin/xo bench/perf/http/cpuburst/run.sh OUT [ROUNDS] [LOOPS...]   # default 2 rounds, 0 8 16 loops
gh workflow run ci.yml -f ref=BRANCH -f suite=script -f packages="bench/perf/http/cpuburst/ci.sh 3"   # on a CI runner (ci.sh: run.sh with XO_CLI into XO_CI_OUT)
```

Each run prints loadgen's failures (resets past the listen queue, 128 on
darwin for Xo and Go alike), dial and dial-to-response percentiles, and
the machine's load average; results.txt in OUT keeps them. Compare
patterns across interleaved rounds, and rerun on a quiet machine (load
under 10) for figures to cite.

First results (2026-10-20, Mac, load 22 to 27, Xo Lang 3): Go drops
connections too under these loads, so the resets are the test's 200
simultaneous SYNs on a busy machine, not an Xo gap. With 8 loops (cores
free) Go's dial-to-response p50 stayed 4 to 6 ms and p99 37 to 46 ms,
Xo A's p50 4 to 21 ms and p99 130 to 147 ms (a request queued behind an
86 ms handler on its worker waits for it), and Xo B was slower than A
(p50 40 to 150 ms). Owner, 2026-10-20: work stealing and back-edge
preemption in one scheduler decision (Xo Lang 2), measured with this.

Main baseline on CI (2026-10-20, run 39465430589: xo-arm64, an r8g on
Linux, load 5 to 7, main of about 12:00, 3 rounds; Linux's listen queue
is 4096, so no connection failed). Dial-to-response p50 / p99 in ms,
rounds 1 and 2:

| loops | Xo A | Xo B | Go |
|---|---|---|---|
| 0 | 0.17 / 2.5, 0.15 / 2.0 | 0.15 / 2.1, 0.19 / 2.3 | 0.44 / 2.9, 0.38 / 2.5 |
| 8 | 60.3 / 85.9, 60.4 / 86.1 | 60.7 / 83.0, 60.4 / 86.1 | 0.55 / 11.5, 0.59 / 11.7 |
| 16 | 60.2 / 86.4, 59.7 / 83.0 | 60.2 / 85.8, 59.6 / 85.6 | 0.98 / 10.3, 0.77 / 9.2 |

This is the baseline the scheduler work (decision 0286, Xo Lang 2) is
measured against on the same runner; it says nothing about the cause.
B matching A does not rule out the root running serve inline: main's
scope_join claims the spawned serve task and runs it on the root as
well (Xo Lang 2's trace).
