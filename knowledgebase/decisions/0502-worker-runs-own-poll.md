# 0502: A worker back from its own poller wait runs what it readied

Status: Accepted (owner, 2026-11-04; condition met: on a quiet linux/arm64 host 1
(load 0.76 at the start), 25 rounds of tcpping handshake give 0.619 (0.607
to 0.749) of main's wall time and 0.914 (0.907 to 0.927) of Go's; smtp send
0.507 (0.505 to 0.526); no HTTP p99 or p999 loss (open loop /alloc c64 at
10,000/s p99 113 against 112.5 us, p999 140.5 both; at 20,000/s 144 and
168 both) and micro13 spawn100k within noise (1.061, 0.958 to 1.128));
proposed by xo-lang-4 the same day.

## Context

bench/perf/tcpping's handshake (a client and a server fiber per connection,
a short request and a short reply) ran 1.54x Go's wall time on linux/arm64
(Lang 3, confirmed with `xo bench --ab`, 2026-11-04). Per connection Xo
makes the same reads, writes and EAGAIN returns as Go's twin; the difference
was hand offs between threads: about 26,000 voluntary context switches in a
run against Go's few hundred, and XO_RTSTATS counted about 11,000 chain
wakes (ST_WAKE_CHAIN), close to one per turn around.

An idle worker waits in the network poller when no one else does (decision
0324, np_block). When that wait returned events, the fibers it readied sat
on the worker's own ring, but go_idle returned 0 ("work showed up") and the
worker counted itself a spinner. Its find_work then took its own fiber, and
as the last spinner that found work it woke another worker (Go's rule for a
spinning M, which keeps one looking while there is work to find). That
worker found nothing and slept again: a futex wake and sleep on every turn
around. Go's thread back from a blocking netpoll in findrunnable runs the
first goroutine on its own P without becoming a spinner and wakes another
only when it readied more (injectglist).

## Decision

go_idle and root_idle return 3 when np_block handled events and at most one
other fiber waits on a descriptor (quiet_peers: the sum over workers of
W_NETWAIT, which pd_wait_ctx raises before a fiber parks and lowers when it
wakes, less the fibers just readied). The worker (or the root) then runs
them not spinning, so no chain wake follows: the one other waiter is the
peer of the request and reply in the program, which only the fibers it runs
can ready. With more waiters, a server's other connections, the worker
looks as a spinner as before and its find wakes one to wait in the poller,
so the next connection's request is taken while this one's handler runs.

A wait that readied more than one fiber still wakes one helper (np_spread),
and the poller's other paths are unchanged, including Lang 1's np_break
clear after the read loop (EV_BRK, 92990de73). The count costs two relaxed
adds on the parking worker's own word per descriptor park and a sum over
the workers only when a poller wait returns events.

## Consequences

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

A program whose fibers talk to each other over a descriptor (a client and
server in one program, a test, a proxy's two sides) no longer pays a futex
wake and sleep per turn around; a server with other connections waiting
behaves as before.

## Alternatives

- Skip the chain wake after every poller wait that readied fibers (this
  decision without quiet_peers): tcpping handshake 0.732 of main and 0.974
  of Go, smtp 0.523, and bench/perf/jitter's open loop /alloc c64 at 20,000
  req/s better everywhere (p99 107 against 143 us, CPU 40 against 57 us a
  request), but at 10,000 req/s p99 141 against 113 us and p999 189 against
  140: no worker waited in the poller while one ran a handler, so the next
  connection's request waited behind it. Not taken: a server's tail
  latency is not traded for a turn around's.
- Skip it only while the work after a poller return averaged under 20 us
  (a moving average of the worker's busy time): the gain went (handshake
  0.99 of main, 1.36 of Go), since tcpping's worker stays busy across
  connections.
- An idle worker polls without waiting for up to XO_NPSPIN_NS before it
  blocks (one spinner at a time, as Go caps its spinning threads): with this
  change, handshake a further 0.689 and smtp 0.723 of wall, but 1.27 to 1.31
  of the CPU and micro13 spawn100k 1.378 slower; without it, httpsession ran
  1.75x slower (the spinner readied the client on its own ring and split the
  pair across threads). Not taken.
- The spin only right after the worker's own fiber parked on a descriptor:
  worse on every row (handshake 3.4x Go).
- np_poll waits out another worker's look without waiting (TOK_WORKER)
  instead of skipping its poll: httpsession went to 13,000 wakes; the token
  was not the cause.
- XO_NETPOLL=0 (only the poller thread waits in epoll): 8x slower.
