# Why /alloc's p50 was higher at 5,000/s than at 20,000/s (2026-11-02, Xo Lang 4)

The open loop numbers of [jitter-encoder-2026-11-02.md](jitter-encoder-2026-11-02.md)
had Xo's `/alloc` p50 at 713 us at 5,000 requests a second and 113 us at
20,000, and Go's hand-written route the same shape (697, 244). Cause: the
load generator, not either server. bench/perf/jitter/loadgen sleeps each
connection to its next send with time.Sleep; when every P of the
generator is idle, Go's runtime waits in epoll_pwait, whose timeout is
whole milliseconds, so a send fires up to about 1 ms late. At 20,000/s a P
is rarely idle and the timers fire on time. The latency counts from the
scheduled send (no coordinated omission), so that lateness was in p50.

linux/arm64 first host (a linuxkit VM: no cpuidle states exposed, no
cpufreq governor; perf_event_paranoid 2 and no root, so no perf sched),
server on CPUs 0-5, generator on 6-11, 64 connections, 3 rounds.

## The generator alone

A probe with the generator's schedule and no network (64 goroutines
sleeping to their next send): lateness p50 / p99 us, median of 3.

| rate/s | time.Sleep | plus one goroutine always runnable | plus nice 19 busy loops on the generator's CPUs |
|---:|---|---|---|
| 5,000 | 624 / 1,524 | 6 / 26 | 583 / 9,991 |
| 10,000 | 552 / 1,461 | 6 / 23 | 576 / 15,782 |
| 20,000 | 302 / 1,377 | 6 / 24 | 650 / 31,011 |

Keeping the CPUs busy does not help, so it is not CPU idle states: only a
runnable goroutine, which keeps the runtime out of its millisecond
netpoll wait, does.

## In the bench

The generator splits each latency into its lateness and the server's part
(svc: from the actual send to the response). p50 us, median of 3, before
the fix:

| rate/s | Xo /alloc p50 (late, svc) | Go /alloc-hand p50 (late, svc) |
|---:|---|---|
| 5,000 | 714 (564, 141) | 698 (495, 150) |
| 10,000 | 608 (420, 163) | 543 (157, 222) |
| 20,000 | 108 (24, 80) | 235 (35, 166) |

Busy loops at nice 19 on the server's or the generator's CPUs are not a
usable control here: CFS still gives them slices, and svc rose to about
1 ms (the scheduler's slice) for both servers.

## Fix

loadgen `-awake` (on by default in the open loop) keeps one goroutine
runnable. With it:

p50 / p99 / p99.9 us, and p50's split (late, svc), median of 3 rounds,
same build and host, `-awake=false` against `-awake=true`:

| rate/s | Xo /alloc before | Xo /alloc with -awake | Go /alloc-hand before | Go /alloc-hand with -awake |
|---:|---|---|---|---|
| 5,000 | 707 / 1,620 / 1,764 (550, 141) | 91 / 142 / 187 (7, 82) | 697 / 1,786 / 2,145 (498, 148) | 98 / 366 / 482 (9, 89) |
| 10,000 | 585 / 1,721 / 5,430 (396, 163) | 92 / 126 / 145 (6, 84) | 547 / 2,188 / 5,318 (164, 222) | 106 / 403 / 515 (9, 95) |
| 20,000 | 114 / 196 / 245 (24, 86) | 106 / 166 / 193 (9, 95) | 251 / 768 / 1,796 (36, 178) | 181 / 662 / 909 (11, 166) |

The server part (svc) fell too at low rates (Xo 141 to 82, Go 148 to 89):
it is measured from the actual send to the response read, and an idle
generator also woke late to read the response. With `-awake`, Xo's p50 is
flat (91 to 106 us) across the rates; no error in any run; server CPU a
request unchanged within the rounds' spread except Xo at 10,000 and 20,000
(below).

## Xo's CPU a request with -awake

Xo's /alloc CPU a request rose with `-awake`: 60.0 to 70.7 us at 20,000 a
second (3 rounds each: 60.05, 60.45, 60.0 against 68.45, 70.7, 71.0), and
about 60 to 64 at 10,000; Go's did not move (127.3 and 125.4 at 20,000).
The server and the generator are on disjoint CPUs (taskset 0-5 and 6-10
or 6-11), and the CPU figure is the server's process alone
(/proc/PID/stat utime + stime, every thread). Separated, 3 rounds at
20,000 a second, the generator on CPUs 6-10:

| run | Xo CPU us a request |
|---|---|
| plain | 59.6, 59.6, 59.6 |
| a busy process (nice 0) on spare CPU 11 | 59.8, 60.0, 60.0 |
| `-awake` | 67.7, 69.0, 69.7 |

A process spinning beside it does not raise the server's CPU, so it is not
the host (a VM) sharing cores. XO_RTSTATS, per request at 20,000 a second,
plain against `-awake`: wakes 0.993 and 0.991, futex waits 1.114 and
1.126, the same; poll returns 1.678 against 2.223, fiber parks 0.774
against 0.983, steals 0.229 against 0.032. A late generator sent in
bunches, and a worker's poll found several requests at once; on schedule,
each request arrives alone and costs its own poll and park. The `-awake`
figures are the even load an open loop means to offer; the older CPU
figures had the bunching in them.

Open loop numbers before this change (jitter-2026-10-30.md,
jitter-encoder-2026-11-02.md) carry the generator's lateness in p50 and
p99 at low rates; their svc columns and the closed loop numbers do not.
