# A shutdown on a loaded fd number can hit the next connection

Status: Observed (2026-11-06)

## Symptom

With the Linux skip-read change (std/changes/http.md, held), the golden
http_reverse_proxy_ws failed in 2 of 3 TestGoldenTasks runs: the second
WebSocket dial through `http.reverse_proxy` got `bad handshake`, the proxy
logging `protocol error: Get "http://127.0.0.1:N/bye": EOF` (a 502), in 4
to 11 of 50 rounds of a repro, never at the parent. The skip-read code
itself returned no wrong result.

## Cause

`hconn.shut_wr` and `hclient.shut_wr` loaded the connection's fd number
and called shutdown(2) without holding the poll descriptor. A close on
another task between the load and the call freed the number; the next
accept or connect took it; the shutdown then half-closed that new,
unrelated connection. An fd log (socket, accept, shutdown, close) showed
it: `close fd=9`, `accept fd=9` (the upstream server's next connection),
then `shutdown fd=9 how=1` from the old tunnel's last `hconn.shut_wr`.
The upstream read EOF before the request and closed; the proxy answered
502. The skip-read change only moved the tunnel's teardown so the close
came first more often.

## Fix

Hold the descriptor around any use of its fd outside the poll module:
`poll.pd_begin(pd)` (false once it is closed) and `poll.pd_end(pd)`
(which closes the fd if a close came meanwhile), as recv_raw and
send_at do. Both shut_wr functions do since 2026-11-06, and so do the
HTTP client's plain reads and writes (hclient fill, send and the body
read), which had the same load-then-use: the tunnel's copy task writes
on the upstream connection while the other side may close it, and a
write into a reused number puts one connection's bytes on another's
(the remaining `handshake status 0`, about 1 round in 100 with or
without skip-read, is measured against this).
