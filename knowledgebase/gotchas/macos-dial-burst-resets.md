# A burst of thousands of dials on macOS loopback resets some connections

Status: Observed (2026-11-17, std/ui rooms load client, decision 0695)

## Symptom

The examples/ui-arena load client opened 2,000 websockets at once
against one server on the Mac. 10 to 11% of the dials failed 40 to 80 ms
in: Xo's websocket client reported a status 0 handshake error (an EOF),
and a Go client dialing the same server saw `connection reset by peer`
(213 and 226 of 2,000). A Go net/http server under the same burst lost
more (540 and 626). `netstat -s` showed no listen queue overflows.

## Cause

macOS caps a listen backlog at `kern.ipc.somaxconn`, 128 by default
(`sysctl kern.ipc.somaxconn`, 2026-11-17), whatever the server asks for.
A burst of dials fills the accept queue faster than the server accepts,
and the kernel resets the connections it has no room for. It is the
host, not the server: both an Xo and a Go server lose connections.

## Fix

Pace the dials of a load client: 1 ms between dials lost none of 4,000
(the arena load client's default, its STAGGER_US argument). Raising
`kern.ipc.somaxconn` needs root and changes the host for every agent, so
do not. Linux defaults to 4096 (`/proc/sys/net/core/somaxconn`, read on
offload host 2, 2026-11-17).
