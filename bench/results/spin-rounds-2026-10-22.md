# Worker spin rounds 0 / 16 / 64 on main (2026-10-22, Xo Lang 2): for the owner's decision

The default is 0 (decision 0077's addendum, owner 2026-10-16: 16 rounds cost
rwmutex 1.27x the CPU; pingpong 0.47x CPU without spinning). Tonight's
measurements on current main (41cbcb183 and later), in the lead's quiet
window; XO_SPIN_ROUNDS sets the rounds at run time, nothing else differs.

## CRUD server, linux/arm64 (Docker, np-linux-cfg, 4 reps, 03:20-03:35, load 7 to 9)

| cores | spin | rps | server CPU us a request | CPU against spin 0 | futex_wait a request |
|---|---|---|---|---|---|
| 1 | 0 | 92,620 | 14.07 | 1.000 | 0.012 |
| 1 | 16 | 90,995 | 14.36 | 1.021 | 0.012 |
| 1 | 64 | 91,115 | 14.31 | 1.017 | 0.012 |
| 3 | 0 | 82,788 | 26.42 | 1.000 | 0.44 |
| 3 | 16 | 90,235 | 24.51 | 0.928 | 0.43 |
| 3 | 64 | 90,472 | 25.43 | 0.963 | 0.45 |
| 6 | 0 | 67,215 | 39.29 | 1.000 | 0.96 |
| 6 | 16 | 69,300 | 40.16 | 1.022 | 1.07 |
| 6 | 64 | 73,276 | 40.59 | 1.033 | 0.96 |

## CRUD server again, second quiet window (05:35-05:48, load median 9.8 with a runaway clang; base interleaved with a rejected lever, 4 reps)

| cores | spin | rps | server CPU us a request | CPU against spin 0 | rps against spin 0 | wake a request |
|---|---|---|---|---|---|---|
| 1 | 0 | 96,267 | 13.49 | 1.000 | 1.000 | 0.0001 |
| 1 | 64 | 89,491 | 14.56 | 1.079 | 0.930 | 0.0002 |
| 3 | 0 | 86,853 | 25.66 | 1.000 | 1.000 | 0.181 |
| 3 | 64 | 84,225 | 26.86 | 1.047 | 0.970 | 0.115 |
| 6 | 0 | 69,619 | 38.89 | 1.000 | 1.000 | 0.573 |
| 6 | 64 | 75,950 | 38.69 | 0.995 | 1.091 | 0.488 |

Under load, 64 rounds cost 1 and 3 cores 3 to 7% throughput and 5 to 8% CPU
and gave 6 cores +9% throughput at the same CPU: the 6 core gain repeats
(first window +9%), the 1 and 3 core rows differ from the first window's.

## bench/perf/http (the CRUD server on the Mac, arm64, 3 interleaved reps of 5 s, 03:47, load 24 falling)

| spin | rps | p50 us | p99 us | p99.9 us | server CPU us a request | CPU against spin 0 |
|---|---|---|---|---|---|---|
| 0 | 125,998 | 217 | 890 | 1,713 | 20.19 | 1.000 |
| 16 | 133,091 | 205 | 845 | 1,613 | 20.71 | 1.026 |
| 64 | 134,387 | 208 | 803 | 1,507 | 21.37 | 1.058 |

## bench/perf/conc (Mac, arm64 backend, 5 interleaved rounds, medians; coarse)

These programs run 10 to 70 ms, at /usr/bin/time's 10 ms resolution, so
only a large change shows: rwmutex CPU 0.10 s (0, 16) and 0.12 s (64,
1.2x); pingpong 0.03 s at all three; mutex 0.01 to 0.02 s (below the
resolution); spawn CPU 0.26 / 0.24 / 0.29 s (1.00 / 0.92 / 1.12). A
decision on the conc rows needs a longer run (bench/perf/conc/run.sh with
more iterations, or instruction counts), in a later quiet window.

## Reading

- 3 cores: 16 or 64 rounds give +9% throughput at 4 to 7% less CPU a
  request; 6 cores: 64 rounds give +9% throughput at +3.3% CPU.
- The Mac's http: 16 rounds +5.6% rps, -5% p99, +2.6% CPU; 64 rounds
  +6.7% rps, -10% p99, +5.8% CPU.
- rwmutex looks 1.2x CPU at 64 (coarse), the regression that set the
  default to 0; 16 rounds show no rwmutex cost here (also coarse).
- Not decided overnight (the owner's decision): a candidate is 16 rounds
  (most of the throughput, CPU within 3% everywhere measured in the first
  window; 16 was not in the second window's row), pending a precise conc row.
  The second window repeats 64's 6 core gain (+9%) but shows 1 and 3 cores
  3 to 7% slower at 64 under load.
