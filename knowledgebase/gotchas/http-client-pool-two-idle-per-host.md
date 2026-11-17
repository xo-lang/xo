# A load client of many simulated users redials on every request: the pool keeps 2 idle connections per host

Status: Observed (2026-11-16, std/ui live-viewers benchmark)

## Symptom

A benchmark ran 1,000 simulated browsers as 10 processes of 100, each
browser with its own `http.client(...)` syncing to one server. On the Mac
most requests dialed a new connection: 100 viewers left 16,157 sockets in
TIME_WAIT, the server spent 11.6 s of system CPU against 1.4 s of user
CPU, and at 1,000 viewers dials failed with `connect: can't assign
requested address` (local ports used up) and viewers fell 17 s behind.
A traced `GotConn` showed 8,320 new connections for 10,002 requests. The
TIME_WAIT sockets were all on the client side.

## Cause

The native client keeps one idle pool per process, shared by every
Client (hclient's pool_put; see
[http-client-pool-scope-differs.md](http-client-pool-scope-differs.md)),
and keeps at most 2 idle HTTP/1.1 connections per host (H1_PER_HOST, Go's
DefaultMaxIdleConnsPerHost) and 100 in all (Go's MaxIdleConns). With 100
users in one process sending to one server, 98 of every 100 finished
connections were closed instead of pooled. A new Client per user does not
help: the pool is per process. Go behaves the same with its default
Transport. Real browsers each keep their own connections, so the
benchmark, not the server, made the churn.

## Fix

A client standing in for many users raises the cap:
`http.client(net).with_max_idle_per_host(N)` with N at least the users per
process (and at most 100 users per process, the pool's total cap). With
it, the same run reused 19,997 of 19,997 connections at 100 viewers, and
at 1,000 viewers every viewer stayed current as the writer finished.
Check a load benchmark with a trace (`Request.with_trace`, counting
`GotConn(reused:)`) or the TIME_WAIT count before trusting its numbers.
