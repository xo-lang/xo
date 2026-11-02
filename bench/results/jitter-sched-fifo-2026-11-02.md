# Steadiness under SCHED_FIFO: Xo and Go 1.26.1 (2026-11-02, Xo Lang 4)

The owner asked (2026-11-02) for the jitter variant with the server on a
real time policy, which jitter-2026-10-30.md could not run (its container
lacked CAP_SYS_NICE). Each server ran twice per configuration: plain
(SCHED_OTHER) and `chrt -f 80` (SCHED_FIFO, priority 80), and each is
compared with itself.

- Host: the second linux/arm64 offload host (18 CPUs, a container),
  with OFF_EXCLUSIVE=1 and OFF_RTPRIO=1 (the job runs as the xo user
  through setpriv with rtprio 99; `chrt -f 1 true` ok;
  sched_rt_runtime_us 950000 of 1000000, the kernel's default throttle).
- The host was not quiet: the 1-minute load average before each run had
  a median of 20.4 (17.0 to 25.1, 240 runs) on 18 CPUs; CPU steal ticks
  0. This file therefore measures SCHED_FIFO under CPU contention from
  other work on the host, not on an idle machine; the plain rows are
  below jitter-2026-10-30.md's quiet rows (Xo /hello c32 249k against
  356k req/s) and must not be compared with them.
- Xo built at main 2d90b65d9 (after the Linux first-chunk fix), Xo's
  native backend; Go's default GOGC. Go's /alloc is its encoding/json
  handler (reflecting), not the hand encoder of jitter-encoder-2026-11-02.md,
  so the Go /alloc rows are not a Xo against Go comparison.
- Servers on CPUs 0-5 (taskset), the load generator on 6-11, with
  `-d 10s -warmup 2s` and `-awake` (on by default in open loop,
  802724c10). 10 rounds, the servers' order alternating per round, plain
  then FIFO for each. Scripts: Xo Lang 4's jf.sh and jf-remote.sh
  (~/xo-lead/notes/xo-lang-4/scripts).
- Medians of 10 rounds, with the range in brackets.

## Xo

| row | policy | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req |
|---|---|---|---|---|---|---|---|
| /hello c32 | plain | 249,481 [199,274 to 252,799] | 104 | 550 | 3,132 | 8,288 | 6.45 |
| /hello c32 | FIFO | 399,164 [375,251 to 409,276] | 57 | 291 | 1,645 | 9,300 | 8.88 |
| /hello c256 | plain | 560,442 [539,842 to 575,977] | 352 | 2,584 | 4,414 | 12,424 | 6.81 |
| /hello c256 | FIFO | 767,044 [730,479 to 778,818] | 274 | 1,408 | 3,904 | 20,974 | 7.15 |
| /alloc c32 | plain | 83,553 [81,437 to 85,904] | 280 | 2,686 | 4,224 | 11,402 | 51.00 |
| /alloc c32 | FIFO | 117,088 [110,475 to 120,689] | 229 | 1,074 | 3,422 | 8,509 | 47.05 |
| /alloc c256 | plain | 108,750 [104,844 to 110,491] | 1,502 | 11,632 | 17,436 | 27,472 | 50.30 |
| /alloc c256 | FIFO | 114,976 [110,560 to 117,497] | 1,563 | 10,556 | 25,588 | 136,332 | 51.10 |
| /alloc open 10k/s | plain | 10,000 | 236 | 4,590 | 7,486 | 11,970 | 83.41 |
| /alloc open 10k/s | FIFO | 10,000 | 197 | 3,746 | 6,556 | 11,132 | 104.85 |
| /alloc open 20k/s | plain | 20,000 | 292 | 5,308 | 8,860 | 14,060 | 69.32 |
| /alloc open 20k/s | FIFO | 20,000 | 198 | 3,698 | 6,310 | 10,638 | 94.05 |

## Go 1.26.1

| row | policy | req/s | p50 us | p99 us | p99.9 us | max us | CPU us/req |
|---|---|---|---|---|---|---|---|
| /hello c32 | plain | 209,891 [205,175 to 216,148] | 87 | 1,661 | 4,748 | 12,730 | 12.39 |
| /hello c32 | FIFO | 286,673 [282,536 to 290,338] | 67 | 959 | 2,619 | 9,000 | 15.30 |
| /hello c256 | plain | 233,248 [228,107 to 238,696] | 462 | 7,927 | 12,506 | 24,588 | 13.19 |
| /hello c256 | FIFO | 365,408 [361,279 to 369,169] | 409 | 3,647 | 5,122 | 12,326 | 13.50 |
| /alloc c32 | plain | 11,027 [10,767 to 11,387] | 1,586 | 14,880 | 22,800 | 38,295 | 192.05 |
| /alloc c32 | FIFO | 15,463 [15,212 to 15,529] | 1,177 | 8,932 | 12,945 | 21,130 | 237.33 |
| /alloc c256 | plain | 21,095 [20,133 to 21,396] | 1,375 | 83,965 | 129,642 | 231,718 | 148.98 |
| /alloc c256 | FIFO | 30,776 [30,049 to 31,385] | 1,254 | 58,132 | 85,834 | 158,214 | 155.76 |
| /alloc open 10k/s | plain | 9,990 | 15,574 | 129,586 | 187,047 | 234,455 | 199.36 |
| /alloc open 10k/s | FIFO | 10,000 | 524 | 6,991 | 11,326 | 20,973 | 286.58 |
| /alloc open 20k/s | plain | 10,597 | 3,216,928 | 5,687,878 | 5,859,820 | 5,963,682 | 192.11 |
| /alloc open 20k/s | FIFO | 16,743 | 1,126,432 | 2,179,077 | 2,346,920 | 2,462,012 | 233.05 |

Go's /alloc with encoding/json cannot carry 20k req/s on 6 contended
CPUs under either policy (it falls behind and the queue grows, so its
latencies there are seconds).

## Findings

- Under contention, FIFO helps both servers in the same way: closed loop
  throughput +37 to +60% for Xo (/hello c32 and c256, /alloc c32), +37
  to +57% for Go (every closed loop row), and p99 1.7 to 2.5 times lower
  on those rows (Go's open loop at 10k: 18 times, its plain run having
  fallen behind). The server's threads stop losing
  their CPUs to the host's other work; the effect is the policy's, not
  either runtime's.
- Xo's /alloc c256 gains least (+5.7% req/s) and its far tail gets worse
  under FIFO: p99.9 25.6 against 17.4 ms and max 136 against 27 ms
  (worst 193 ms), the one row where FIFO hurts. A likely cause, not
  checked here: with every worker thread at priority 80 and more runnable
  fibers than CPUs, FIFO has no time slice, so a thread is not preempted
  for a waiter of equal priority and a stall lasts until it blocks.
- CPU per request rises under FIFO in open loop (Xo +26% at 10k and
  +36% at 20k; Go +44% and +21%). A likely cause, not checked here: a
  FIFO thread that spins before it parks keeps its CPU through the spin
  instead of losing it to other work, so the spin is paid in full.
- Recommendation: no change to Xo. SCHED_FIFO is a deployment choice
  that pays where a server shares its CPUs with other work, with two
  costs measured here: more CPU per request at moderate rates, and a
  worse far tail when connections outnumber CPUs by many. A quiet host
  run (the first offload host, alone) is the open item; on an idle
  host the policy has little to take back.
