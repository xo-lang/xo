# A worker back from its poller wait runs what it readied (decision 0502), 2026-11-04

Run by xo-lang-4 (tcpfull-remote.sh: Xo at A against Xo at B on tcpping,
smtp, httpsession and micro13 spawn100k, the programs built from B's
sources by each xo with its own std, then Go's twins against B; the HTTP
rows by scripts/release-vs-go-http.sh with A's xo as the base).

## The decision (quiet_peers)

`xo bench --ab --no-guard --rounds=9 --warmup=1`, linux/arm64 offload host 1,
2026-11-04: main bef074fef (A) against it plus this change (B), each
program built from the same sources by each xo and its std
(bench/results/worker-own-poll-2026-11-04.md). Host load 0.60 at the start,
a median of 5.6 to 7.3 during the run from other jobs on the host.

| row | wall B/A | cpu B/A |
|---|---|---|
| tcpping handshake | 0.828 (0.737 to 1.03) within noise | 0.869 (0.803 to 0.972) better |
| tcpping connect | 0.779 (0.767 to 0.829) better | 0.745 better |
| tcpping pingpong | 0.995 (0.992 to 1.0) | 0.995 within noise |
| smtp send | 0.530 (0.516 to 0.535) better | 0.487 better |
| httpsession store | 1.019 (0.964 to 1.027) within noise | 1.022 within noise |
| httpsession plain | 1.004 (0.985 to 1.019) within noise | 1.003 within noise |
| micro13 spawn100k | 0.997 (0.895 to 1.128) within noise | 0.971 within noise |
| Go (A) vs Xo (B): tcpping handshake | 0.930 (0.922 to 0.945) better | 0.854 better |
| Go (A) vs Xo (B): smtp send | 0.711 (0.699 to 0.719) better | 0.668 better |

Before, handshake was 1.542 (1.52 to 1.566) of Go's wall time on that host.

HTTP (bench/perf/jitter through scripts/release-vs-go-http.sh, four pads,
three rounds, pads.py's verdict on req/s; host load 2.9 at the start):

| row | req/s B/A | verdict | p50 us A / B | p99 us A / B | CPU us/req A / B |
|---|---|---|---|---|---|
| /hello c32 | 1.001 | placement | | | 7.37 / 7.35 |
| /hello c256 | 0.998 | placement | | | 5.93 / 5.96 |
| /alloc c32 | 1.011 | faster | | | 34.54 / 34.11 |
| /alloc c256 | 1.011 | placement | | | 33.54 / 33.18 |
| /alloc c64 open 10,000/s | 1.000 | (rate fixed) | 79 / 78 | 115.5 / 113.5 | 52.3 / 52.0 |
| /alloc c64 open 20,000/s | 1.000 | (rate fixed) | 91 / 91 | 143.5 / 143 | 57.7 / 57.2 |

## The owner's condition: quiet host re-run (2026-11-04, 2e9810489 against bef074fef)

Host load 1.51 at the start (3 to 4.7 median during, much of it the bench's
own threads); the handshake row again by itself, 25 rounds, load 0.76 at
the start.

| row | wall B/A | cpu B/A |
|---|---|---|
| tcpping handshake, 25 rounds | 0.619 (0.607 to 0.749) better | 0.539 (0.521 to 0.805) better |
| Go vs Xo: tcpping handshake, 25 rounds | 0.914 (0.907 to 0.927) better | 0.840 better |
| tcpping handshake, 9 rounds | 0.922 (0.719 to 1.131) within noise | 0.978 within noise |
| tcpping connect | 0.879 (0.75 to 1.111) within noise | 0.728 (0.709 to 0.776) better |
| tcpping pingpong | 0.995 within noise | 0.996 within noise |
| smtp send | 0.507 (0.505 to 0.526) better | 0.470 better |
| httpsession store / plain | 0.993 / 0.990 within noise | 0.995 / 0.987 |
| micro13 spawn100k | 1.061 (0.958 to 1.128) within noise | 1.023 within noise |
| Go vs Xo: smtp send | 0.673 (0.666 to 0.678) better | 0.632 better |

HTTP (four pads): req/s B/A /hello c32 1.000, c256 1.003, /alloc c32 1.005,
c256 1.011 (placement); open loop /alloc c64, A / B: 10,000/s p50 79 / 78,
p99 113 / 112.5, p999 140.5 / 140.5, CPU 52.3 / 51.9 us; 20,000/s p50 91 /
91, p99 144 / 144, p999 168 / 168, CPU 57.8 / 57.6 us.

## Skipping the chain wake after every poller return (not taken)

main 80af16b4c against it plus the change without quiet_peers; host load
0.48 at the start, 6 to 8 during.

| row | wall B/A |
|---|---|
| tcpping handshake | 0.732 (0.687 to 0.902) |
| smtp send | 0.523 (0.519 to 0.533) |
| Go vs Xo: tcpping handshake | 0.974 |
| Go vs Xo: smtp send | 0.767 |
| pingpong, httpsession, spawn100k | within noise |

HTTP open loop /alloc c64, medians over four pads and three rounds:

| rate | p50 A / B | p99 A / B | p999 A / B | CPU us/req A / B |
|---|---|---|---|---|
| 10,000/s | 79 / 73.5 | 113 / 141 | 140.5 / 189 | 52.1 / 42.6 |
| 20,000/s | 91 / 75 | 143 / 107 | 167 / 135 | 57.3 / 40.0 |

The 10,000/s tail is why quiet_peers keeps a worker waiting in the poller
while other connections wait. Go's twin on the same rows: p99 391 and 649 us.

## The busy time average (not taken)

Skipping the wake only while the work after a poller return averaged under
20 us: tcpping handshake 0.99 of main and 1.358 of Go, smtp 0.823; HTTP equal
to main.

## The spin (XO_NPSPIN_NS=20000, not taken)

With the chain fix: tcpping handshake 0.689 wall but 1.272 CPU, smtp 0.723
and 1.305 CPU, micro13 spawn100k 1.378 slower. Without it, httpsession 1.75x
slower.

## XO_RTSTATS, one run each (tcpping handshake)

| | wake | wake_chain | voluntary switches |
|---|---|---|---|
| before | 11,169 | 11,169 | 26,670 |
| skip every time | 3,643 | 1,749 | 5,738 |
