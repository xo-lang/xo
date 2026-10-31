# Worker polling (decision 0324) against the poller thread and Go, linux/arm64 (2026-10-30, Xo lang 5)

The owner's "measure first" on decision 0301 (2026-10-30), which 0324
already built and turned on: the CRUD server of bench/perf/http (its
loadgen's POST, GET, PATCH, GET, DELETE cycle, plus GET /users every
eighth), as Go 1.26.1 (`go`), Xo with worker polling, the default (`xo`),
and Xo with `XO_NETPOLL=0`, the poller thread path (`xo-np0`): the same
binary, built by xo at 68ab0dc7b.

- Host: the linux/arm64 offload host (12 CPUs, a container; the job held
  its lock, so nothing else ran). Server on CPUs 0-5 (taskset; Xo's worker
  count follows the affinity mask), load generator on 6-11.
- 20 rounds of 10 s after 2 s of warmup at each of 32 and 256 connections,
  the three servers in a rotating order each round. CPU steal ticks were 0
  in every run; the load average was 8 to 9 (the run's own server and
  client).
- Futex and scheduler counts are XO_RTSTATS's, per request over the whole
  process (warmup and preload included). The host has no strace (the job
  runs unprivileged); 0301 measured strace's own slowdown as most of its
  2.9 futex calls a request.

Medians of 20 rounds:

| conc | server | req/s | p50 us | p99 us | p99.9 us | CPU us/req | futex wait/req | futex wake/req | worker wakes/req | idle sleeps/req | waits in the poller/req | steals/req | inject/req |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 32 | go | 182,614 | 127 | 891 | 1,246 | 18.62 | | | | | | | |
| 32 | xo | 219,486 | 120 | 703 | 1,211 | 13.62 | 0.142 | 0.169 | 0.096 | 0.090 | 0.071 | 0.278 | 0.0000 |
| 32 | xo-np0 | 186,717 | 155 | 686 | 1,116 | 12.87 | 0.165 | 0.189 | 0.128 | 0.122 | 0.044 | 0.130 | 0.544 |
| 256 | go | 196,235 | 584 | 5,062 | 6,899 | 20.66 | | | | | | | |
| 256 | xo | 224,798 | 1,174 | 3,768 | 5,620 | 16.01 | 0.200 | 0.249 | 0.071 | 0.064 | 0.039 | 0.338 | 0.0000 |
| 256 | xo-np0 | 226,512 | 1,084 | 3,538 | 5,358 | 15.00 | 0.139 | 0.171 | 0.058 | 0.054 | 0.008 | 0.172 | 0.399 |

Paired by round (each ratio's median, and the rounds on one side):

| conc | xo / xo-np0 req/s | xo / xo-np0 p99 | xo / go req/s | xo / go p99 |
|---|---|---|---|---|
| 32 | 1.176 (20 of 20 above 1) | 1.027 | 1.200 (20 of 20) | 0.796 |
| 256 | 0.992 (0 of 20 above 1) | 1.059 | 1.144 (20 of 20) | 0.742 |

## Reading

- Against Go, at both levels and in every round: 14 to 20% more requests
  a second, p99 20 to 26% lower, CPU a request 23 to 27% lower. Go's p50
  at 256 connections is lower (584 against 1,174 us): Go serves each
  connection's next request sooner, Xo's tail is shorter.
- Worker polling against the poller thread: at 32 connections +17.6%
  requests a second in every round, p50 155 to 120 us, futex calls -12%,
  for +5.8% CPU a request; at 256 connections 0.8% fewer requests a second
  in every round, p99 +6%, CPU +6.7%, futex calls +45%: with every worker
  busy, the fibers one poll readies land on that worker's ring and the
  others steal them (steals 0.17 to 0.34 a request), 0324's Docker
  finding at 3 and 6 cores again.
- 0301's expected saving, the poller's wake and the worker's sleep merged:
  inject goes from 0.40 to 0.54 a request to 0, worker wakes -25% at 32
  connections. The futex calls a request were already 0.14 to 0.20, not
  2.9; 0301's figure was strace's.
- A first pass (10 rounds, earlier the same evening) swung 3x within every server,
  Go's too (54k to 145k requests a second at 32 connections), on the same
  host with nothing else holding its lock; it is not used. Its CPU a
  request agreed with this run's within 7%.
- Next for the 256 connection case: hand a poll's second and later
  fibers to idle workers' rings directly (0324 tried waking one worker a
  fiber: inject fell but wakes and futex calls rose 12 to 19%), or cap
  the fibers a poll keeps on its own ring at what it can run before the
  next poll.

Raw lines: ~/xo-lead/notes/xo-lang-5/l5np2-2026-10-30.txt (first pass: l5np-2026-10-30.txt).
