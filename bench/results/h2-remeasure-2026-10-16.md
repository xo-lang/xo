# HTTP/2 against the C base, remeasured (2026-10-16)

The zero-C sweep (bench/results/zero-c-perf-sweep-2026-10-15.md) found
HTTP/2 over TLS 1.12 to 1.85 times main's instructions on the client and
1.39 to 1.45 times its CPU per request on the server, with AES-GCM
(`aesgcm.hw4`) the hot spot. Remeasured on main at cf1f3c31 (which has
5ff3e546's four block GHASH and 64 byte CTR) against main at dd33dd13
(the C runtime with Mbed TLS, xo 0.11.0, built from its own tree), on the
development Mac (Apple silicon, macOS 27.0) with other sessions running,
under the machine-wide heavy lock. No fix was needed: every ratio is
within 5% of 1 or below it.

## Client (bench/perf/httpclient)

`WORKLOADS="h2len h2big" measure.sh XO OUT arm64 llvm`, N=3000 requests on
one pooled HTTP/2 connection over TLS to the Go serve/ on loopback, run
twice per side, interleaved (base, new, then new, base); each run is the
minimum of 5 (`/usr/bin/time -l` instructions retired of the client).
The figure is the minimum over both runs.

| Workload | Backend | dd33dd13 | cf1f3c31 | Ratio | 2026-10-15 |
|---|---|---|---|---|---|
| h2len | arm64 | 616.02M | 628.14M | 1.020 | 1.12 |
| h2len | llvm | 598.68M | 575.99M | 0.962 | 1.037 |
| h2big | arm64 | 5352.82M | 5248.75M | 0.981 | 1.852 |
| h2big | llvm | 5332.85M | 4649.66M | 0.872 | 1.440 |

Each side's two runs: h2len arm64 616.39M, 616.02M / 628.14M, 635.02M;
h2len llvm 600.77M, 598.68M / 575.99M, 576.37M; h2big arm64 5407.22M,
5352.82M / 5248.75M, 5297.43M; h2big llvm 5408.37M, 5332.85M /
4760.06M, 4649.66M.

## Server (bench/perf/http)

`PORT=18380 SCHEME=https H2=1 REPS=5 DURATION=5s WARMUP=1s
CONFIGS="xo-base-arm64 xo-arm64 xo-base-llvm xo-llvm" run.sh`: the task
01 server over TLS with HTTP/2, 32 workers as streams of one connection,
configs interleaved 5 times; medians. (`xo-base-arm64` is new in run.sh
for this run: the base xo with `--backend=arm64`.)

| Config | Req/s | p50 us | p99 us | CPU us/req | Ratio (CPU) |
|---|---|---|---|---|---|
| xo-base-arm64 | 70551 | 429 | 1041 | 41.59 | |
| xo-arm64 | 72746 | 414 | 1013 | 41.21 | 0.991 |
| xo-base-llvm | 67752 | 446 | 1086 | 40.03 | |
| xo-llvm | 70951 | 428 | 981 | 39.37 | 0.984 |

The sweep's server figures (29.2 against 42.2 us on arm64) were taken
with its own driver at a lower load per run; both sides here ran at
about 70k requests per second, and the per run spread was wide (CPU
us/req 33.29 to 42.49 across all runs, the first round lower on every
config).
