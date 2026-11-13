# Loopback stress exhausts a shared host's ephemeral ports

Status: Observed (2026-11-13)

## Symptom

Lang 3's certpool gate on linux/arm64 host 2 failed its llvm step with
"ephemeral ports exhausted: 28176 of 28232 in TIME_WAIT" while
std/evidence/http/stress ran there under `stdproven race`. On a Mac, 64
client tasks of that program left 16247 sockets in TIME_WAIT in 30 s and
then failed themselves with `connect: can't assign requested address`.

## Cause

std/http's client keeps at most 2 idle HTTP/1.1 connections per address
(hclient.xo H1_PER_HOST, Go's DefaultMaxIdleConnsPerHost), in one pool
for the process. 64 tasks sending to one address at once returned 64
connections to a pool that keeps 2, so about 62 closed after every
request. Each close leaves its client side in TIME_WAIT for 60 s (Linux),
so tens of thousands of requests per second used up the 28232 ports of
the host's range, for every program on the host, not only the stress.
Go's DefaultTransport behaves the same with as many goroutines.

## Fix

The stress program gives the server one listener per two client tasks,
so each address has no more than 2 clients and their connections stay
pooled: TIME_WAIT after 30 s went from 16247 to 265 (the hostile
clients' connections, about 9 a second, which close on purpose).

## Rule

A loopback load or stress program on a shared host keeps its clients'
connections alive: no more concurrent clients per address than the
client pools (2 for std/http, Go's default), or a pool sized for them.
A program that churns connections on purpose stays under about 400 new
connections a second per host, or runs on a lease that takes the whole
host.
