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

## Profile on host 1 and the open gap

Rerun after the lighter in-twin server (9 rounds, load 0.97): send 186.6
against 254.4 ms = 1.364 (1.347-1.421), unchanged from 1.375, so the
server's line scan was not the gap. XO_PROF cpu on host 1 at one worker
thread, 40,000 messages (505 samples): `conn_write` 27.7% flat,
`futex_wake` 15.1%, `poll_round` 8.1%, `conn_read` 7.5%, `pd_close` 7.3%,
`dial_timeout` 6.9%, `listener_accept` 4.8%, `epoll_add` 3.6%; smtp.run 2%
flat (23% cumulative, nearly all in those calls). The time is in the
system calls, not in smtp code, and the same calls cost less in tcpping:
the difference is the call mix of a message (15 small writes and 11 small
reads across 11 turn arounds on a new connection each), which tcpping's
two shapes do not reproduce together. The host has no strace and the job
runs unprivileged, so the syscall sequence of Go's SendMail against Xo's
smtp.send is not yet compared.

Next experiments, ranked: (1) tcpping with 11 turn arounds on a new
connection each message (`handshake`: dial, 11 request/reply pairs of 20
bytes, close), which is the smtp shape without smtp; if it shows 1.3, the
cause is the per connection plus many small turn arounds mix and the
poll/park path is the lever (Lang 1, scheduler); if it shows 1.0, the cause
is inside smtp.send (then bisect with a client only run against a Go
server process). (2) `futex_wake` at 15% with one worker: who wakes whom
(XO_RTSTATS counted 151 wakes in 2,000 messages, so each is expensive or
the attribution is skewed): perf is not available, so a run with
`XO_THREADS=1` and the idle sleep counts per message.

## Result of experiment 1: the smtp shape without smtp (host 1, 9 rounds)

tcpping `handshake` (2,000 connections, each 11 turn arounds of 20 bytes,
then close): Go 173.5, Xo 264.7 ms = 1.525 (1.498-1.569). So the smtp gap
is not in smtp.send; it is the net and scheduler path of many small turn
arounds on short lived connections. Per round trip (time over round trips):

| shape | Go us/trip | Xo us/trip |
|---|---|---|
| pingpong, one connection, 60,000 trips | 10.1 | 10.0 |
| handshake, 2,000 connections x 11 trips | 7.9 | 12.0 |
| connect, 4,000 connections x 1 trip | 114 per connection | 53 per connection |

Xo's time per trip is steady (10 to 12 us) while Go's is 8 on short lived
connections (the client and server goroutines share a P) and 10 on one long
one. The lever is the per turnaround cost of a park and wake pair in
Xo's poll path (Lang 1's scheduler: 11 parks, 21 epoll returns and 10.6
EAGAIN reads a message): `xo bench --ab` on tcpping handshake is the gate
for any change there.

# std/sql row decode, linux/arm64 host 2 (7 rounds, n=6)

sqlrow scan: Go (stub driver) 56.53, Xo (std/sql.memory) 91.77: 1.623 of
Go (1.568-1.651). The `none` row (8.218) is the setup only: Xo inserts 1,000
rows through exec, Go has no table to fill, so it is not a comparison.
Xo's side includes the in memory engine's SELECT and ORDER BY, Go's stub
hands out rows, so the gap is an upper bound on the decode cost; the pgx
row above (1.24) is the like for like one.

## Round trip against connect: the tcpping twin (host 1, 5 rounds)

bench/perf/tcpping (a persistent connection echoing 60,000 messages of 64
bytes, and 4,000 connections that each make one round trip and close),
Go first, Xo / Go:

| workload | Go | Xo | Xo / Go | range |
|---|---|---|---|---|
| pingpong (60,000 round trips) | 606.0 | 598.3 | 0.987 | 0.709-1.166 (noisy) |
| connect (4,000 connections) | 456.4 | 212.2 | 0.465 | 0.390-0.970 |

The I/O path itself is at Go's speed or better (connect is twice as fast),
so the 21 poll returns and 10.6 EAGAIN reads a message above are not the
smtp gap by themselves, and the question for Lang 1 shrinks to whether
anything in them is avoidable, not whether it explains smtp. What differs
in the smtp twin: its in-twin servers (Xo's scanned bytes in a loop, Go's
bufio), which made half of the Xo profile (`serve` 48% cumulative of 25
samples on host 1) before the server's line scan used Bytes.find.
