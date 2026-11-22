# An idle connection parked after the server's resumer had ended

Status: Observed (2026-11-22, xo-4c found it as task #666, Xo Lang 1 fixed it as task #671).

Symptom: a program with an http server in a background task, a request
from a client in the same scope, then the scope's end, was sometimes
still running long after its output: `twincheck -time 5` of
bench/perf/grpcreflect called it a hang (4 of 4 rounds on linux/arm64
host 1), and a minimal h2c server stalled in 1 run of 450 to 2 of 92
there, never on a loaded host. Every worker slept on its futex, the
poller thread in epoll_wait, `xo debug tasks` showed the server's task
`blocked in serve (canceled)`, and one connection's sockets were still
open. It looked like a lost wakeup, and a signal seemed to end it.

Cause: not a lost wakeup. When serve's scope is canceled, serve waits
for its connections to end for at most 30 s (hserve.xo srv_wait), and
the program ended by itself after that. The first check of the "hang"
waited 20 s, attached a debugger, then ran `xo dump` with a 10 s
timeout: past 30 s, so the dump's signal got the credit. The
connection that stayed open had parked as idle (decision 0449: no task
holds it, the resumer task resumes or closes it). On the cancel the
resumer ends in drain, which closes what is parked then; this
connection had read "not canceled" just before and parked after the
drain and before stop_at marked the server stopping. hc_park already
refused a park after a stop, for the same reason, but not after the
drain. An HTTP/2 connection parks the moment its response is sent
(HTTP/1.1 lingers 20 ms first), which is when the client's scope ends.

Fix: drain marks the server (SR_NORESUME) under the lock hc_park
holds; a park after it is refused, and one that the drain passed
between its listing and its pd_park takes itself back. The connection
reads on, finds the cancel, and ends.

How to avoid: a "hang" with a server in it: time it with no signal
and no debugger before reading the scheduler (30 s is srv_wait). To
prove an ordering a run hits once in hundreds, hold the two steps
apart in a scratch build (here a spin of 60 ms before the park and of
400 ms before the stop: 31 s every run before the fix, 1 s after). A
record with no task of its own needs an owner on every path that ends
the owner it had.
