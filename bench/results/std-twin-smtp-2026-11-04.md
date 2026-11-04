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
(XO_WORKERS=1 or the scheduler's equivalent) to see the same-thread
handoff, then `xo bench --ab` of any scheduler change on this twin.
