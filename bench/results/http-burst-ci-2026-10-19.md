# Connection bursts: native Xo against Go on a quiet linux/arm64 machine (2026-10-19)

Owner question (2026-10-19, one CI run approved): is the native Xo HTTP
server more sensitive than Go's to bursts of connections? In Docker on
the busy development Mac, both stalled 0.3 to 1.5 s in some rounds
(gotchas/docker-burst-stalls-on-loaded-mac.md), which pointed at the
Linux VM being descheduled, not at either server.

## Setup

CI run 39322714355 (script suite, `bench/perf/http/burst-ci.sh` from
branch burst-ci at 14a3f274, tested merged with main; main included
4538689f, the fiber stack retention of fiber-stack-cache-2026-10-19.md),
on a self-hosted linux/arm64 runner: 16 CPUs, 124 GB, load average 5.90
at the start. The servers ran on CPUs 0-7 and loadgen on 8-15
(taskset); somaxconn 4096, tcp_max_syn_backlog 4096. Six alternating
blocks (go, xo, go, xo, ...) of 50 rounds, each round 2,000 connections
dialed at once, one `GET /users/1` each with `Connection: close`
(`loadgen -burst 2000`). The xo was 0.21.0, built by the suite from the
seed. The artifact's burst.txt:

| server | rounds | wall p50 | wall p99 | wall max | over 150 ms | over 300 ms | latency p99 (worst block) | failed connections | minor faults |
|---|---|---|---|---|---|---|---|---|---|
| go | 300 | 14.7 ms | 227.3 ms | 335.1 ms | 6 | 1 | 91.7 ms | 0 | 118018 |
| xo | 300 | 14.1 ms | 225.6 ms | 280.5 ms | 6 | 0 | 116.6 ms | 0 | 12663 |

Per connection latency p99 by block (connect to first response line):

| block | 1 | 2 | 3 | 4 | 5 | 6 |
|---|---|---|---|---|---|---|
| go | 64.2 ms | 91.7 ms | 87.1 ms | 65.5 ms | 85.6 ms | 85.3 ms |
| xo | 116.6 ms | 96.2 ms | 116.0 ms | 96.2 ms | 96.1 ms | 85.4 ms |

## Result

- On a quiet machine the native server is not more stall prone than
  Go's: the same number of slow rounds (6 over 150 ms each), a lower
  maximum (280.5 against 335.1 ms), and no failed connections for
  either. The Mac's stalls were the Docker VM; the gotcha now says so.
- With the fiber stack retention on main, Xo's minor faults over 300
  rounds were 12,663 against Go's 118,018.
- Open: the per connection latency p99 inside a block is higher for Xo
  (85 to 117 ms against Go's 64 to 92) while the median is equal (9.7
  against 9.5 ms) and the round's wall time is lower. That is the order
  in which accepted connections get their first read under a burst
  (accept to first read scheduling), not throughput. Left for later; a
  run of burst-ci.sh reproduces it, and no further CI run is planned.
