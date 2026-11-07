# Skipping a read on an edge-triggered poller misses a FIN that came with the data

Status: Observed (2026-11-07)

Symptom: from 1c9bc8e5f (2026-11-06) internal/llvm `go test -run 'Diff$'`
ran for hours on linux/arm64 with no output, stuck in TestHTTP1ParseDiff:
its senders sat in io.ReadAll until their 30 second deadline, connection
after connection (2000 inputs, three backends, 16 at a time). It looked
like a hang from the gate's newest change (first the Go parity sample,
then an unrelated std/rt commit); the same command at main hung too.
macOS was not affected (the skip is Linux only).
Bisect (host 2, TestHTTP1ParseDiff with XO_HTTP_DIFF_N=300): 1c9bc8e5f
failed with 15 `<timeout>` answers on the native backends in 269 s; its
parent e778eb397 passed in 44 s with none.

Cause: 1c9bc8e5f made the read after a short read (one that returned
fewer bytes than asked, taken to mean the socket is drained) wait for a
poller edge first, unless an edge came since. On an edge-triggered
epoll, data and the sender's FIN that arrive before the wait come as one
edge. The wait took it, the short read took the data, and the end of
stream was left for the next read; no new edge comes for it, so that read
waited until the peer gave up. It takes bytes left in the server's buffer
after a request (pipelined requests, a body followed by more bytes,
trailing garbage): with nothing buffered the connection parks between
requests, and parking clears the mark, so a lone half-closed request was
answered (the first TestHTTP1HalfClose, one GET per connection, passed
without the fix). A load test of keep-alive clients never half-closes,
so its measurements (and the http goldens) never hit it.

Fix (2026-11-07): the poller keeps a hangup mark per descriptor (D_RHUP,
set on EPOLLRDHUP, EPOLLHUP or EPOLLERR, never cleared), and the skip
does not apply once it is set (poll.pd_read_ready). Any change that
skips a syscall on the strength of "no edge since" must account for
events that one edge covers together; test it with a sender that
half-closes right after writing pipelined requests (TestHTTP1HalfClose).
