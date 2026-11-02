# A socket registered before its connect wakes the dial as if connected

Status: Observed (2026-11-02, linux/arm64 and linux/amd64, under load),
fixed in std/native/llrt/netconn.xo connect_wait.

Symptom: golden net_lookup_dial_timeout printed "blackhole: connected?"
in 3 of 256 runs on linux/arm64 and 1 of 256 on linux/amd64, with 64
copies at once beside CPU burners: `dial_timeout` to 192.0.2.1 (an
unroutable test address) returned a connection instead of Timeout. Never
seen in a plain golden run.

Cause: two facts together.
1. dial_timeout calls poll.pd_open (epoll_ctl ADD, edge triggered, read
   and write) before connect. Linux reports a TCP socket that is not
   connecting yet as EPOLLOUT | EPOLLHUP (checked on the second offload
   host with Python's select.epoll: mask 20 before connect, nothing once
   the connect is in progress, because epoll re-polls the socket when it
   reports the event).
2. connect_wait took READY with SO_ERROR 0 as connected. SO_ERROR is
   also 0 while a connect is in progress.
When the poller thread happened to collect the registration's event
between the ADD and the connect, pd's write readiness was set, the
first wait returned at once, and the dial "connected".

Fix: Go's check (net/fd_unix.go connect): after a write wake with
SO_ERROR 0, getpeername; if it fails the connect has not finished, so
wait for the next edge. Clearing the readiness after connect instead
could drop the real completion edge, which an edge triggered poller
never repeats. Test: netconn_test.xo "a writable wake before the connect
finished is not a connection" hands pd the edge with poll.pd_ready
before an unconnected socket's connect_wait.

Avoid: a readiness flag says an event came, not that the operation it
was for finished. Confirm the state with the kernel (SO_ERROR plus
getpeername for connect, EAGAIN for reads) before acting on a wake.
