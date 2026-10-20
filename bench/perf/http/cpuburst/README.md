# cpuburst: connection bursts while handlers are CPU bound

A small server (Xo in xo/, Go in go/) with GET /users/1 and GET /cpu
(about 86 ms of compute, no runtime calls), and run.sh, which measures
loadgen's burst mode (200 connections at once, 10 rounds) while N curl
loops keep /cpu busy. Xo runs twice: A serves in main's body (as every
server does), B in a spawned task (main parked at the scope's end). Go
is the reference.

```
XO=bin/xo bench/perf/http/cpuburst/run.sh OUT [ROUNDS] [LOOPS...]   # default 2 rounds, 0 8 16 loops
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
