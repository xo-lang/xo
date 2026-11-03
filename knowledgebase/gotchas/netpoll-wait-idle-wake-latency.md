# A netpoller wait costs about 1 ms per wake when the scheduler is idle

Status: Observed (2026-11-03, Xo Lang 1)

Symptom: std/dns lookups of one query moved from ppoll(2) on the worker
thread to the netpoller (poll.pd_new, pd_wait, as netconn.udp_read waits;
31ba6938b) ran 0.205 of the time in bench/perf/dns mx (an in-process DNS
server task) but about twice as long against the host's real resolver on
linux/arm64: 600 MX lookups 0.57 to 0.62 s with ppoll, 1.05 to 1.26 s
with the netpoller (about 1 ms more per lookup). A and AAAA lookups on
the netpoller (two sockets) were 3 to 5% slower with 2 to 3 times the
system time. Reverted.

Cause: under investigation. With nothing else to run, the task parks, the
worker goes idle, and the readiness the poller thread sees reaches the
task about 1 ms later (a sleep or tick granularity in the idle path, or
the deadline timer's re-arm). In the in-process bench the server task
kept the workers busy, so the wake was a fiber switch.

Fix: none yet. Measure a netpoller change against a peer outside the
program as well as an in-process one: a benchmark whose server is a task
of the same program hides the idle wake path.
