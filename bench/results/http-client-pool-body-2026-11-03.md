# HTTP client: pooling after a request with a body source (2026-11-03)

The change: a connection that served a request whose body came from a
source (Request.with_body_reader) goes back to the pool once the response
was read to its end and the body was sent in full, as net/http's
Transport does (persistConn's wroteRequest). Before, Client.open's full
duplex path (decision 0433) closed it every time.

Program: bench/perf/httppost, 2000 keep-alive POSTs of a 34-byte body
source against a std/http server in the same process. `xo bench --ab`
on the first linux/arm64 offload host (12 CPUs, Linux aarch64, xo 0.37.0),
3 rounds.

`open` (Client.open, the full duplex path), A the parent 1d525c62a and
B 6ad3d5a9c, the pushed code with the 50 ms wait for the sender's mark:

| metric | A median | B median | B / A | 95% interval | verdict |
|---|---|---|---|---|---|
| wall_ms | 120.8 | 24.04 | 0.199 | 0.194 to 0.225 | better |
| cpu_ms | 199.8 | 24.72 | 0.124 | 0.119 to 0.143 | better |
| max_rss_kb | 3888 | 2940 | 0.756 | 0.69 to 0.839 | better |

Before the wait (A 2daf4f48c, B 2b761fc75, before a rebase): wall 0.253
(0.231 to 0.314), cpu 0.16, RSS 0.769. Without the wait the golden dialed
a second connection on linux/arm64 when the reader finished the response
before the sender stored its mark; with it, 30 of 30 runs used one.

`send` (Client.send, which already pooled such a connection; A
2daf4f48c, B 2b761fc75):

| metric | A median | B median | B / A | 95% interval | verdict |
|---|---|---|---|---|---|
| wall_ms | 22.42 | 23.31 | 1.04 | 0.953 to 1.1 | within noise |

Load 0.95 to 0.98 in every round, none loaded. Connections counted by
golden http_client_pool_body: 6 POSTs with body sources and 2 GETs on 1
connection, where the native client dialed 4 (main's std at 98f71e696);
the Go backend gives 1.
