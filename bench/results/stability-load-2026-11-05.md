# Stability load: notes against its Go twin, 2026-11-05

Last verified: 2026-11-05

Decision 0520 phase 1, check 2 (load). `stability/harness load` at 0285ea4d7 drove
stability/notes, built with xo for the arm64 native backend, and its Go twin
bench/size/native/go/notes. Both ran with the journal off (`-drop-env NOTES_DATA`; the twin
keeps no journal). There were 3 rounds with the order alternating, each run on a fresh start,
with a 5 s warmup and 30 s measured. The host was linux/arm64 offload host 1 (12 CPUs), with
the client on the same host. Verdicts follow bench/perf/jitter's pads.py rule: better or
worse only when the 3 runs' ranges do not overlap.

Reproduce: `~/xo-lead/notes/xo-lang-2/stability/load-run.sh 0285ea4d7 -d 30s -warmup 5s
-rounds 3 -rates 1000,4000 -sat 64 -drop-env NOTES_DATA`. The lead's script bundles the
revision to the host, builds xo there, and runs the harness command shown in
stability/README.md.

## Results (median, range of 3 runs)

| load | metric | xo arm64 | Go twin | xo/go | verdict |
|---|---|---|---|---|---|
| 1000/s offered | req/s | 995.6 | 996.2 | 0.999 | offered |
| | p50 us | 148 (144 to 176) | 172 (165 to 178) | 0.860 | ranges overlap |
| | p99 us | 580 (496 to 589) | 710 (671 to 761) | 0.817 | better |
| | p99.9 us | 1,068 (916 to 1,081) | 1,168 (1,129 to 1,265) | 0.914 | better |
| | CPU us/req | 103.1 (95.4 to 114.5) | 156.9 (154.9 to 157.8) | 0.657 | better |
| | peak RSS KiB | 21,244 | 50,808 (50,672 to 52,880) | 0.418 | better |
| 4000/s offered | req/s | 3,985.8 | 3,986.5 | 1.000 | offered |
| | p50 us | 234 (181 to 234) | 159 (156 to 165) | 1.472 | **worse** |
| | p99 us | 942 (812 to 971) | 848 (771 to 855) | 1.111 | ranges overlap |
| | p99.9 us | 2,232 (2,099 to 2,234) | 1,375 (1,202 to 1,552) | 1.623 | **worse** |
| | CPU us/req | 83.5 (67.6 to 84.2) | 96.7 (92.5 to 97.5) | 0.863 | better |
| | peak RSS KiB | 21,244 (21,244 to 24,408) | 52,832 (52,736 to 53,016) | 0.402 | better |
| saturation, 64 closed loop workers | req/s | 26,330 (25,534 to 27,324) | 9,817 (9,609 to 10,292) | 2.682 | better |
| | p50 us | 1,767 (1,722 to 1,780) | 5,923 (5,851 to 6,158) | 0.298 | better |
| | p99 us | 13,402 (12,715 to 14,109) | 23,189 (22,599 to 25,348) | 0.578 | better |
| | p99.9 us | 21,157 (20,118 to 22,839) | 31,613 (31,489 to 35,218) | 0.669 | better |
| | CPU us/req | 67.6 (65.9 to 68.7) | 171.6 (164.0 to 172.2) | 0.394 | better |
| | peak RSS KiB | 41,160 (39,028 to 43,368) | 52,852 (52,776 to 52,856) | 0.779 | better |

No run had errors or drops.

## The one loss: latency at 4000/s

At 4000/s offered, Xo's median latency is 1.47 times Go's and its p99.9 1.62 times, with
less CPU per request (0.86). A run of the same build with XO_RTSTATS=1 (counters over the
process's life, 5 s warmup plus 30 s, median of 3 runs) shows a thread woken from idle for
about every request at both fixed rates:

| load | wake | fiber_park | steal | futex_wait | poll_wake | wakes per request |
|---|---|---|---|---|---|---|
| 1000/s | 35,875 | 36,804 | 5,237 | 79,758 | 34,723 | about 1.0 |
| 4000/s | 129,568 | 219,187 | 97,270 | 176,271 | 138,764 | about 0.93 |
| saturation | 1,225,798 | 2,087,829 | 915,051 | 1,361,839 | 908,316 | about 1.4 |

The hypothesis is that Xo's idle threads sleep at once: XO_SPIN_ROUNDS defaults to 0 since
decision 0077's addendum. Each request at a moderate rate then pays a futex wake, where Go's
threads spin briefly before parking. The first experiment is the same load with
XO_SPIN_ROUNDS=16; it is recorded below when done.

The XO_SPIN_ROUNDS=16 experiment ruled spinning out (same command with `-env XO_SPIN_ROUNDS=16`, 3 rounds).
At 4000/s Xo's p50 was 229 us (129 to 233), against Go's 166 (161 to 193): the same as
without spinning (234). Its CPU per request fell to 47.8 us (0.48 of Go), and every
latency range overlapped. That run was noisier: Go's own p50 at 1000/s was 92 us,
against 172 in the run above, and Xo's p99 at 1000/s ranged from 249 to 2,582 us. So
whether a thread wakes from idle is not what makes the 4000/s median slow.

The two apps do not do the same work. The Go twin keeps notes in a map behind a
sync.Mutex. The Xo app goes through std/sql's in-memory driver, which parses each
statement and holds the database's lock while it runs. Xo's p50 rising with the rate
(148 us at 1000/s, 234 at 4000/s) while Go's stays flat (172, 159) fits a serialized
section that is held longer. The next experiment is an XO_PROF=cpu profile of the Xo
build at 4000/s, plus a run of a Go twin on the same std/sql-shaped store, to tell the
app's storage apart from the runtime.

## Notes

- An earlier run at 63a9235fc offered 4000/s but sent about 1,200/s: the harness's open loop
  lost time.Ticker ticks under a millisecond (gotchas/go-ticker-submillisecond-rate.md, fixed
  in 0285ea4d7). Only its saturation rows, which are closed loop, compare: xo 25,879 req/s
  against 8,949 (2.89x).
- The Xo build's peak RSS at the fixed rates is the same 21,244 KiB in nearly every run: the
  heap reaches its working size during the warmup.
