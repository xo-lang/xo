# Connection bursts in Docker on a loaded Mac stall the whole container

Status: Observed (2026-10-19, Xo Lang 3).

## Symptom

`loadgen -burst 2000 -reps N` (bench/perf/http/loadgen burst.go) against a
server in scripts/docker-linux.sh's linux/arm64 container: most rounds
answer all 2,000 connections in 40 to 60 ms, but some take 0.3 to 1.5 s.
It happens with the native Xo server and the Go server alike (8
alternating blocks of 150 rounds at a Mac load average of 45 to 80: rounds
over 150 ms, Go 15 and Xo 35; over 300 ms, Go 6 and Xo 15; maxima 865 ms
and 1.3 s), and the counts follow the Mac's load, not the server.

## Cause

The late part of a stalled round is connect(), not the server: about
half the connections take 0.3 to 0.85 s to connect, all completing in one
batch, while every connection's time from connect to response stays under
50 ms. The kernel dropped nothing (ListenOverflows, ListenDrops,
TCPReqQFullDrop, TCPSynRetrans all 0 in /proc/net/netstat), and a shell
loop sampling the server's threads every 10 ms inside the container
stopped for the same 150 to 180 ms: the whole Linux VM was not running
(its vCPUs descheduled by macOS under the other sessions' load). With the
server and the client on separate CPUs (taskset -c 0-5 and 6-11) the
native server matched Go (p99 68 ms against 63 ms, no stall), and the
accept drain itself is equal (2000 x 50: wall p50 47.7 ms against 47.6
ms).

## Fix

None in Xo. Measure accept bursts with the client and server on
separate CPUs, compare servers in alternating blocks, and judge stalls on
a quiet Linux machine, not in Docker on a busy Mac. A gate test that
fails on a round over 10 times the median would fail for Go here.

Found on the way (a separate matter, not the stall): the native fiber
cache keeps 8 stacks per worker and 64 shared (std/native/llrt/pool.xo
FCACHE, FIBER_CACHE), so a burst of 2,000 connections maps and unmaps
about 1,840 fiber stacks each round: 116,000 minor faults per 150 rounds
against 2,500 with a cache of 4,096 (and 5 to 7% less server CPU).
