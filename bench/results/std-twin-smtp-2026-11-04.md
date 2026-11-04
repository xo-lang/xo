# std/smtp against Go's net/smtp (2026-11-04)

Twin bench/perf/smtp (2,000 messages of about 1 KB, 2 recipients, one
connection each, to an in-process loopback server; checksums equal,
twincheck). linux/arm64 host 1, 7 rounds (n=6), Go first:

| workload | Go | Xo | Xo / Go | range |
|---|---|---|---|---|
| send | 184.6 | 253.9 | 1.375 | 1.353-1.399 |
| none | 0.982 | 0.542 | 0.552 | 0.507-0.677 |

Where the time goes (Mac, XO_PROF cpu, 6,000 messages): 54% in
`_ulock_wait` and 21% in `kevent` of the 920 ms sampled, with the connect
and send calls at 5% each: the run is bound by the 11 round trips a message
makes between the client task and the server task, not by the SMTP code
(no smtp function in the top 14). The gap is the cost of a handoff between
two fibers on different workers (a wake through the scheduler's park and
the poller) against Go's same-P goroutine handoff; the client code itself
is not the lever. Next experiment: the same run with one worker
(XO_THREADS=1) to see the same-thread
handoff, then `xo bench --ab` of any scheduler change on this twin.

## One worker thread (host 1, XO_THREADS=1 and GOMAXPROCS=1)

| | Go | Xo | Xo / Go |
|---|---|---|---|
| send, 12 threads (above) | 184.6 | 253.9 | 1.375 |
| send, 1 thread | 93.93 | 217.7 | 2.318 (2.289-2.389) |

Go halves its time on one thread (the server goroutine runs as soon as the
client parks); Xo gains 14%. getrusage of one run each (host 1, 2,000
messages): Go 1 thread user 0.037 sys 0.057 s, 71 voluntary context switches;
Xo 1 thread user 0.062 sys 0.189 s, 16,378 voluntary context switches (12
threads: Go sys 0.141, Xo 0.222). XO_RTSTATS of Xo at 1 thread, per message:
11 fiber parks, 25 net_read calls of which 10.6 net_again (EAGAIN), 15
net_write, 21 poll returns (epoll_wait), 9.4 poll events, futex_wait 0.17.
So the futex handoff between threads is not the cost (the 12 thread run has
3.3 futex waits a message, yet is only 17% slower than 1 thread); the cost
is kernel time in the I/O path itself: 95 us of system time a message
against Go's 28.

Next experiments, ranked:
1. A persistent-connection ping-pong twin (bench/perf/tcpping: 11 round
   trips a message without connect, accept and close) to split the round
   trip cost from the connection cost; compare sys time and XO_RTSTATS
   per round trip.
2. If the round trip is the gap: one epoll_wait per fiber park (21 poll
   returns against 11 parks) is the candidate: batch the poll with the
   next run queue pass (Lang 1's scheduler; measure with `xo bench --ab`).
3. If connect and close are the gap: per-connection syscalls (setsockopt,
   getsockname, getpeername, shutdown) that Go skips.

# std/sql row decode, linux/arm64 host 2 (7 rounds, n=6)

sqlrow scan: Go (stub driver) 56.53, Xo (std/sql.memory) 91.77: 1.623 of
Go (1.568-1.651). The `none` row (8.218) is the setup only: Xo inserts 1,000
rows through exec, Go has no table to fill, so it is not a comparison.
Xo's side includes the in memory engine's SELECT and ORDER BY, Go's stub
hands out rows, so the gap is an upper bound on the decode cost; the pgx
row above (1.24) is the like for like one.
