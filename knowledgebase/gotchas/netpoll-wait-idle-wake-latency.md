# A netpoller wait cost about 1 ms per wake in a program with no concurrency

Status: Observed (2026-11-03, Xo Lang 1), fixed

Symptom: std/dns lookups of one query moved from ppoll(2) on the worker
thread to the netpoller (poll.pd_new, pd_wait, as netconn.udp_read waits;
31ba6938b) ran 0.205 of the time in bench/perf/dns mx (an in-process DNS
server task) but about twice as long against the host's real resolver on
linux/arm64: 600 MX lookups 0.57 to 0.62 s with ppoll, 1.05 to 1.26 s
with the netpoller (about 1 ms more per lookup). A and AAAA lookups on
the netpoller (two sockets) were 3 to 5% slower with 2 to 3 times the
system time. Reverted.

Cause: the program had no concurrency in its code as the compiler
counts it (concflag.xo, ssa/conc.xo: scopes, tasks, channels, sleeps, the
prelude's Net and Conn methods, stdin, processes; std/dns's lookups are
not on those lists), so main ran on its own thread with no root loop and
no workers. The poller was still set up for decision 0324's mode, where
idle workers poll the network and the poller thread only ticks
(poll_main, NP_TICK_MIN 0.5 ms and up): with no worker, every wake waited
for the next tick. A program with any spawn in its code (even one never
run) ran its lookups at full speed, which is how it was found:
cross-process responder, 5000 lookups, 0.15 s with ppoll, 4.1 s on the
netpoller with no spawn in the code, 0.18 s with one; XO_NETPOLL=0
0.23 s.

Fix: poll.xo np_wanted is false when concflag.built() is (the poller
thread waits in epoll or kqueue itself, as before decision 0324): 4.1 s
to 0.19 s for that program.

Check: measure a netpoller change against a peer outside the program as
well as an in-process one, and with a program that has no concurrency in
its code: a benchmark whose server is a task of the same program keeps
the workers busy and hides the idle wake path.
