# HTTP/2 instruction counts varied 5% between identical callgrind runs

Status: Observed (2026-10-17, bench/perf/http/callgrind.sh's h2
scenario, Xo Lang 3)

Symptom: the h2 scenario's instructions per request moved 5% to 7%
between repeats of the same server binary (arm64 170,948 to 179,483),
while plain HTTP/1.1 and TLS stayed within 0.3%. A per function diff of
two repeats put the difference in tlssrv read, flush, recv_some, fill,
and save, and in the scheduler (park_wait, pd_wait, steal_round): the
server made a different number of read attempts per request.

Cause: two races between the client and the server under valgrind.

1. net/http's HTTP/2 client flushes a request's HEADERS and then its
   DATA separately (h2_bundle.go writeRequestBody: a flush per DATA
   frame "for latency"), and sends WINDOW_UPDATE and SETTINGS acks as
   their own writes. Whether they reach the server in one read or
   several depends on the goroutines' timing.
2. After answering, the server's connection fiber reads again without
   waiting (tlssrv.nb_read): when the client's next request is already
   there it reads it, else it gets -2, parks, wakes, and reads again,
   about 4,000 instructions more. Whether the next request beat the
   server there is timing too.

Fix: loadgen -h2det (bench/perf/http/loadgen/h2det.go) speaks HTTP/2
by prior knowledge over a TLS connection it dials itself and holds the
client's writes until a frame ends a stream, so each request and the
control frames before it are one TLS record; -gap 20ms pauses before
each request in -route mode, so the server is always parked when it
comes. callgrind.sh's h2 scenario passes both. Repeats now stay within
0.23% (arm64) and 0.14% (LLVM). Either one alone left a 1.5% to 2%
spread. The counts are higher than before (the parked path every
request) and are not comparable with h2 numbers measured before
2026-10-17.

XO_THREADS=1 alone narrowed the spread to about 2% and changes what is
measured; it is not used.
